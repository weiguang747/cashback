package database

import (
	"context"
	"os"
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

	rows, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: a.id})
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
