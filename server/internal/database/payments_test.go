package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/db"
)

// payingUser commits a user whose IDR main account holds balanceMinor, and
// removes everything it created when the test ends. Pay commits its own
// database transactions, so these tests can't run inside a rolled-back one.
func payingUser(t *testing.T, pool *pgxpool.Pool, balanceMinor int64) testUser {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	u := createUser(t, q, fmt.Sprintf("pay-%d@test.example", time.Now().UnixNano()), "IDR")
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM cashback_pending WHERE user_id = $1", u.id)
		pool.Exec(ctx, "DELETE FROM cashback_awards WHERE user_id = $1", u.id)
		pool.Exec(ctx, "DELETE FROM transactions WHERE account_id IN ($1, $2)", u.main, u.cashback)
		pool.Exec(ctx, "DELETE FROM accounts WHERE user_id = $1", u.id)
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", u.id)
	})
	insert(t, q, db.InsertTransactionParams{AccountID: u.main, Kind: "in", AmountMinor: balanceMinor, Currency: "IDR"})
	if err := q.SyncAccountBalance(ctx, u.main); err != nil {
		t.Fatal(err)
	}
	return u
}

func balanceOf(t *testing.T, pool *pgxpool.Pool, accountID string) int64 {
	t.Helper()
	b, err := db.New(pool).GetAccountBalance(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertReconciled(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	drift, err := db.New(pool).ReconcileAccounts(context.Background())
	if err != nil || len(drift) != 0 {
		t.Errorf("stored balances drifted from transactions: %+v, %v", drift, err)
	}
}

// Ten payments of 30 race against a balance of 100: exactly three succeed,
// the rest are declined, and the balance never goes below zero.
func TestPayConcurrentNeverOverdraws(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 100)

	var wg sync.WaitGroup
	var mu sync.Mutex
	paid, declined := 0, 0
	for i := range 10 {
		wg.Go(func() {
			_, err := Pay(context.Background(), pool, PayParams{
				UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 30, MerchantName: "Kopi",
				IdempotencyKey: fmt.Sprintf("concurrent-%d", i),
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				paid++
			case errors.Is(err, ErrInsufficientFunds):
				declined++
			default:
				t.Errorf("pay %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if paid != 3 || declined != 7 {
		t.Errorf("paid %d, declined %d; want 3 and 7", paid, declined)
	}
	if b := balanceOf(t, pool, u.main); b != 10 {
		t.Errorf("balance = %d, want 10", b)
	}
	assertReconciled(t, pool)
}

// Without Redis, the unique key still stops a retry from charging twice,
// including two identical requests racing each other.
func TestPayIdempotencyBackstop(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	u := payingUser(t, pool, 1_000)
	p := PayParams{UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 300, MerchantName: "Kopi", IdempotencyKey: "same-key-1"}

	var wg sync.WaitGroup
	results := make([]PayResult, 5)
	errs := make([]error, 5)
	for i := range 5 {
		wg.Go(func() { results[i], errs[i] = Pay(ctx, pool, p) })
	}
	wg.Wait()

	fresh := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("pay %d: %v", i, err)
		}
		if !results[i].Replayed {
			fresh++
		}
		if results[i].Transaction.ID != results[0].Transaction.ID {
			t.Errorf("pay %d returned transaction %s, want %s", i, results[i].Transaction.ID, results[0].Transaction.ID)
		}
	}
	if fresh != 1 {
		t.Errorf("%d payments charged, want exactly 1", fresh)
	}
	if b := balanceOf(t, pool, u.main); b != 700 {
		t.Errorf("balance = %d, want 700 (one debit)", b)
	}

	// The same key for a different amount is refused, not replayed.
	p.AmountMinor = 500
	if _, err := Pay(ctx, pool, p); !errors.Is(err, ErrKeyReused) {
		t.Errorf("reused key = %v, want ErrKeyReused", err)
	}
	if b := balanceOf(t, pool, u.main); b != 700 {
		t.Errorf("balance = %d after a refused reuse, want 700", b)
	}
	assertReconciled(t, pool)
}

func TestPayInsufficientFundsChangesNothing(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 100)

	_, err := Pay(context.Background(), pool, PayParams{UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 101, MerchantName: "Kopi", IdempotencyKey: "too-much-1"})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
	if b := balanceOf(t, pool, u.main); b != 100 {
		t.Errorf("balance = %d, want 100", b)
	}
	rows, _ := db.New(pool).ListTransactions(context.Background(), db.ListTransactionsParams{UserID: u.id, Limit: 100})
	if len(rows) != 1 {
		t.Errorf("got %d transactions, want only the opening one", len(rows))
	}
}
