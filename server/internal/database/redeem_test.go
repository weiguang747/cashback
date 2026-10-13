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

// earnCashback launches a campaign and pays enough to put earned cash back
// on the user's cash back account.
func earnCashback(t *testing.T, pool *pgxpool.Pool, u testUser, payments int) {
	t.Helper()
	launch(t, pool, launchParams(1_000_000_000))
	for range payments {
		payAt(t, pool, u, 3_500_000, noon) // earns Rp 1.750 each
	}
}

func redeemNow(pool *pgxpool.Pool, u testUser, amount int64, key string) (RedeemResult, error) {
	return Redeem(context.Background(), pool, RedeemParams{UserID: u.id, Currency: "IDR", AmountMinor: amount, IdempotencyKey: key})
}

func TestRedeemMovesCashbackToMain(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 1_000_000_000)
	earnCashback(t, pool, u, 2) // Rp 3.500 of cash back
	mainBefore := balanceOf(t, pool, u.main)

	r, err := redeemNow(pool, u, 350_000, "redeem-all")
	if err != nil {
		t.Fatal(err)
	}
	if r.CashbackBalanceMinor != 0 || r.MainBalanceMinor != mainBefore+350_000 {
		t.Errorf("result = %+v, want cash back 0 and main +350000", r)
	}
	if r.Transaction.Kind != "cashback" || r.Transaction.AmountMinor != 350_000 {
		t.Errorf("main credit = %+v", r.Transaction)
	}
	// The result is built without reading the row back; it must match it.
	key := "redeem-all"
	stored, err := db.New(pool).GetTransactionByIdempotencyKey(context.Background(),
		db.GetTransactionByIdempotencyKeyParams{AccountID: u.main, IdempotencyKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	if r.Transaction.ID != stored.ID || !r.Transaction.CreatedAt.Equal(stored.CreatedAt) ||
		r.Transaction.Name != stored.Name || r.Transaction.Description != stored.Description ||
		r.Transaction.Currency != stored.Currency || r.Transaction.MinorUnit != stored.MinorUnit {
		t.Errorf("returned credit = %+v\nstored         %+v", r.Transaction, stored)
	}
	if b := balanceOf(t, pool, u.cashback); b != 0 {
		t.Errorf("cash back balance = %d, want 0", b)
	}

	// The cash back history shows the redemption as money out.
	history, _ := db.New(pool).ListCashbackTransactions(context.Background(), db.ListCashbackTransactionsParams{UserID: u.id, Currency: "IDR", Limit: 100})
	if len(history) == 0 || history[0].AmountMinor != -350_000 || history[0].Name != "Cash back redeemed" {
		t.Errorf("latest cash back row = %+v, want -350000 'Cash back redeemed'", history)
	}
	assertLedger(t, pool)
}

func TestRedeemPartialAndOverdraw(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 1_000_000_000)
	earnCashback(t, pool, u, 2)

	if r, err := redeemNow(pool, u, 100_000, "redeem-part"); err != nil || r.CashbackBalanceMinor != 250_000 {
		t.Fatalf("partial redeem = %+v, %v; want 250000 left", r, err)
	}
	if _, err := redeemNow(pool, u, 250_100, "redeem-too-much"); !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("over-redeem = %v, want ErrInsufficientFunds", err)
	}
	if b := balanceOf(t, pool, u.cashback); b != 250_000 {
		t.Errorf("cash back balance = %d after a refused redeem, want 250000", b)
	}
	assertLedger(t, pool)
}

// Without Redis, the unique key still stops a retry from redeeming twice,
// including identical requests racing each other.
func TestRedeemIdempotencyBackstop(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 1_000_000_000)
	earnCashback(t, pool, u, 2)

	var wg sync.WaitGroup
	results := make([]RedeemResult, 5)
	errs := make([]error, 5)
	for i := range 5 {
		wg.Go(func() { results[i], errs[i] = redeemNow(pool, u, 175_000, "redeem-once") })
	}
	wg.Wait()

	fresh := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("redeem %d: %v", i, err)
		}
		if !results[i].Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Errorf("%d redemptions moved money, want exactly 1", fresh)
	}
	if b := balanceOf(t, pool, u.cashback); b != 175_000 {
		t.Errorf("cash back balance = %d, want 175000 (one redemption)", b)
	}
	if _, err := redeemNow(pool, u, 100_000, "redeem-once"); !errors.Is(err, ErrKeyReused) {
		t.Errorf("same key, other amount = %v, want ErrKeyReused", err)
	}
	assertLedger(t, pool)
}

// Payments (which credit cash back) and redemptions (which debit it) run
// at the same time without deadlocking or losing money.
func TestRedeemWhileEarning(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 10_000_000_000)
	earnCashback(t, pool, u, 1) // Rp 1.750 to start

	var wg sync.WaitGroup
	var mu sync.Mutex
	redeemed := int64(0)
	for i := range 10 {
		wg.Go(func() {
			_, err := Pay(context.Background(), pool, PayParams{
				UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000,
				MerchantName: "Kopi Senja", IdempotencyKey: fmt.Sprintf("earn-%d", i), Now: noon,
			})
			if err != nil {
				t.Errorf("pay %d: %v", i, err)
			}
		})
		wg.Go(func() {
			_, err := redeemNow(pool, u, 10_000, fmt.Sprintf("spend-%d", i)) // Rp 100 each
			switch {
			case err == nil:
				mu.Lock()
				redeemed += 10_000
				mu.Unlock()
			case !errors.Is(err, ErrInsufficientFunds):
				t.Errorf("redeem %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	// 11 awards of Rp 1.750 in total, minus what was redeemed.
	if b := balanceOf(t, pool, u.cashback); b != 11*175_000-redeemed {
		t.Errorf("cash back balance = %d, want %d", b, 11*175_000-redeemed)
	}
	assertLedger(t, pool)
}

func TestRedeemWithoutCashbackAccount(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 1_000)
	_, err := Redeem(context.Background(), pool, RedeemParams{UserID: u.id, Currency: "USD", AmountMinor: 1, IdempotencyKey: "no-usd-" + time.Now().Format("150405")})
	if err == nil {
		t.Error("redeem in a currency with no accounts succeeded")
	}
}
