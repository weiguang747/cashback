package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/db"
)

// These run against a real Postgres only when TEST_DATABASE_URL is set, e.g.
//
//	TEST_DATABASE_URL=postgres://cashback:cashback@localhost:5432/cashback_test?sslmode=disable go test ./...
//
// Each test works inside a database transaction that is rolled back.

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func begin(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(context.Background()) })
	return tx
}

type testUser struct{ id, main, cashback string }

// createUser makes a user with a main and a cash back account in currency.
func createUser(t *testing.T, q *db.Queries, email, currency string) testUser {
	t.Helper()
	ctx := context.Background()
	id, err := q.UpsertUser(ctx, db.UpsertUserParams{Email: email, Name: email})
	if err != nil {
		t.Fatal(err)
	}
	u := testUser{id: id}
	for _, kind := range []string{"main", "cashback"} {
		acc, err := q.UpsertAccount(ctx, db.UpsertAccountParams{UserID: id, Kind: kind, Currency: currency})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "main" {
			u.main = acc
		} else {
			u.cashback = acc
		}
	}
	return u
}

func insert(t *testing.T, q *db.Queries, p db.InsertTransactionParams) {
	t.Helper()
	if p.Name == "" {
		p.Name, p.Description = "x", "x"
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if _, err := q.InsertTransaction(context.Background(), p); err != nil {
		t.Fatalf("insert %+v: %v", p, err)
	}
}

func TestOpenPoolSize(t *testing.T) {
	pool := openTestDB(t)
	if got := pool.Config().MaxConns; got != DefaultMaxConns {
		t.Errorf("MaxConns = %d, want the default %d", got, DefaultMaxConns)
	}

	// pool_max_conns in the URL wins.
	sep := "?"
	if strings.Contains(os.Getenv("TEST_DATABASE_URL"), "?") {
		sep = "&"
	}
	small, err := Open(context.Background(), os.Getenv("TEST_DATABASE_URL")+sep+"pool_max_conns=3")
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	if got := small.Config().MaxConns; got != 3 {
		t.Errorf("MaxConns with pool_max_conns=3 = %d, want 3", got)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := openTestDB(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestListTransactionsScopesByUser(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	q := db.New(begin(t, pool))

	a := createUser(t, q, "a@test.example", "USD")
	b := createUser(t, q, "b@test.example", "USD")

	// Above JS's Number.MAX_SAFE_INTEGER (2^53 - 1): must survive exactly.
	const big = int64(9_007_199_254_740_993)
	at := time.Now().Truncate(time.Microsecond)
	insert(t, q, db.InsertTransactionParams{AccountID: a.main, Kind: "in", AmountMinor: big, Currency: "USD", CreatedAt: at})
	insert(t, q, db.InsertTransactionParams{AccountID: a.cashback, Kind: "cashback", AmountMinor: 500, Currency: "USD"})
	insert(t, q, db.InsertTransactionParams{AccountID: b.main, Kind: "in", AmountMinor: 100, Currency: "USD"})

	rows, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: a.id, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	// Only a's main account: not b's row, not a's cash back row.
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if r := rows[0]; r.AmountMinor != big || r.Currency != "USD" || r.MinorUnit != 2 || !r.CreatedAt.Equal(at) {
		t.Errorf("round trip = %+v", r)
	}
}

// Paging by (created_at, id) cursor visits every row once, newest first,
// including rows that share a timestamp across a page break, and the
// optional filters narrow it.
func TestListTransactionsPaging(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	q := db.New(begin(t, pool))
	u := createUser(t, q, "paging@test.example", "IDR")

	base := time.Now().Truncate(time.Microsecond)
	for i := range 7 {
		// Rows 2, 3 and 4 share one moment.
		at := base.Add(-time.Duration(min(i, 2)+max(i-4, 0)) * time.Minute)
		name := fmt.Sprintf("Shop %d", i)
		if i == 6 {
			name = "50% off_sale"
		}
		kind, amount := "out", int64(-100)
		if i%3 == 0 {
			kind, amount = "in", 100
		}
		insert(t, q, db.InsertTransactionParams{AccountID: u.main, Name: name, Description: "x", Kind: kind, AmountMinor: amount, Currency: "IDR", CreatedAt: at})
	}
	all, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: u.id, Limit: 100})
	if err != nil || len(all) != 7 {
		t.Fatalf("all: %d rows, %v", len(all), err)
	}

	var paged []string
	params := db.ListTransactionsParams{UserID: u.id, Limit: 3}
	for {
		rows, err := q.ListTransactions(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			paged = append(paged, r.ID)
		}
		if len(rows) < 3 {
			break
		}
		last := rows[len(rows)-1]
		params.BeforeCreatedAt, params.BeforeID = &last.CreatedAt, &last.ID
	}
	if len(paged) != len(all) {
		t.Fatalf("paged %d rows, want %d", len(paged), len(all))
	}
	for i := range all {
		if paged[i] != all[i].ID {
			t.Errorf("row %d = %s, want %s", i, paged[i], all[i].ID)
		}
		if i > 0 && (all[i].CreatedAt.After(all[i-1].CreatedAt) ||
			(all[i].CreatedAt.Equal(all[i-1].CreatedAt) && all[i].ID > all[i-1].ID)) {
			t.Errorf("row %d is not older than row %d", i, i-1)
		}
	}

	in := "in"
	rows, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: u.id, Limit: 100, Kind: &in})
	if err != nil || len(rows) != 3 {
		t.Errorf("kind=in: %d rows, want 3 (%v)", len(rows), err)
	}
	// Wildcards in the search text are escaped by the handler; here the
	// escaped pattern must match only the literal text.
	for pattern, want := range map[string]int{`%50\% OFF\_%`: 1, `%50\%\_%`: 0, `%shop%`: 6} {
		rows, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: u.id, Limit: 100, NamePattern: &pattern})
		if err != nil || len(rows) != want {
			t.Errorf("pattern %q: %d rows, want %d (%v)", pattern, len(rows), want, err)
		}
	}
}

func TestStoredBalancesReconcile(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	q := db.New(begin(t, pool))

	u := createUser(t, q, "c@test.example", "IDR")
	insert(t, q, db.InsertTransactionParams{AccountID: u.main, Kind: "in", AmountMinor: 1_000_000, Currency: "IDR"})
	insert(t, q, db.InsertTransactionParams{AccountID: u.main, Kind: "out", AmountMinor: -350_000, Currency: "IDR"})

	// The insert alone leaves the stored balance behind.
	if drift, err := q.ReconcileAccounts(ctx); err != nil || len(drift) != 1 {
		t.Fatalf("before sync: drift = %+v, err = %v; want 1 account", drift, err)
	}
	for _, id := range []string{u.main, u.cashback} {
		if err := q.SyncAccountBalance(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if drift, err := q.ReconcileAccounts(ctx); err != nil || len(drift) != 0 {
		t.Fatalf("after sync: drift = %+v, err = %v; want none", drift, err)
	}

	accounts, err := q.ListAccounts(ctx, u.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].Kind != "main" || accounts[0].BalanceMinor != 650_000 || accounts[1].BalanceMinor != 0 {
		t.Errorf("accounts = %+v, want main 650000 then cashback 0", accounts)
	}
}

func TestConstraints(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()

	// Each case gets a fresh user and transaction; the statement must fail.
	tests := []struct {
		name string
		run  func(q *db.Queries, tx pgx.Tx, u testUser) error
	}{
		{"money in with negative amount", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			return insertErr(q, db.InsertTransactionParams{AccountID: u.main, Kind: "in", AmountMinor: -100, Currency: "IDR"})
		}},
		{"money out with positive amount", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			return insertErr(q, db.InsertTransactionParams{AccountID: u.main, Kind: "out", AmountMinor: 100, Currency: "IDR"})
		}},
		{"zero amount", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			return insertErr(q, db.InsertTransactionParams{AccountID: u.main, Kind: "in", AmountMinor: 0, Currency: "IDR"})
		}},
		{"unknown kind", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			return insertErr(q, db.InsertTransactionParams{AccountID: u.main, Kind: "refund", AmountMinor: 100, Currency: "IDR"})
		}},
		{"unknown account", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			return insertErr(q, db.InsertTransactionParams{AccountID: "00000000-0000-0000-0000-000000000000", Kind: "in", AmountMinor: 100, Currency: "IDR"})
		}},
		{"currency differs from account", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			return insertErr(q, db.InsertTransactionParams{AccountID: u.main, Kind: "in", AmountMinor: 100, Currency: "USD"})
		}},
		{"negative stored balance", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			if err := insertErr(q, db.InsertTransactionParams{AccountID: u.main, Kind: "out", AmountMinor: -100, Currency: "IDR"}); err != nil {
				return nil // the insert itself must succeed; only the balance may not go negative
			}
			return q.SyncAccountBalance(ctx, u.main)
		}},
		{"duplicate account", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			// UpsertAccount would return the existing row; a plain insert must fail.
			_, err := tx.Exec(ctx, "INSERT INTO accounts (user_id, kind, currency) VALUES ($1, 'main', 'IDR')", u.id)
			return err
		}},
		{"uppercase email", func(q *db.Queries, tx pgx.Tx, u testUser) error {
			_, err := q.UpsertUser(ctx, db.UpsertUserParams{Email: "Upper@test.example", Name: "x"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := begin(t, pool)
			q := db.New(tx)
			u := createUser(t, q, "d@test.example", "IDR")
			if err := tt.run(q, tx, u); err == nil {
				t.Error("succeeded, want constraint violation")
			}
		})
	}
}

func insertErr(q *db.Queries, p db.InsertTransactionParams) error {
	p.Name, p.Description, p.CreatedAt = "x", "x", time.Now()
	_, err := q.InsertTransaction(context.Background(), p)
	return err
}
