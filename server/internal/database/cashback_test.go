package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/cashback"
	"cashback/server/internal/db"
	"cashback/server/internal/dbtrace"
)

var jakarta = time.FixedZone("WIB", 7*60*60)

// The launch campaign's rules in sen: 5%, Rp 20.000 minimum, Rp 50.000 a
// day, whole-rupiah awards.
func launchParams(budgetMinor int64) LaunchParams {
	return LaunchParams{
		Name: fmt.Sprintf("test-%d", time.Now().UnixNano()), Currency: "IDR", RateBps: 500,
		MinSpendMinor: 2_000_000, DailyCapMinor: 5_000_000, AwardIncrementMinor: 100,
		BudgetMinor: budgetMinor, Timezone: "Asia/Jakarta",
		StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// launch starts a campaign and removes it, its account and its funding legs
// when the test ends.
func launch(t *testing.T, pool *pgxpool.Pool, p LaunchParams) (id, accountID string) {
	t.Helper()
	ctx := context.Background()
	id, err := LaunchCampaign(ctx, pool, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT account_id FROM campaigns WHERE id = $1", id).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM cashback_pending WHERE campaign_id = $1", id)
		pool.Exec(ctx, "DELETE FROM cashback_awards WHERE campaign_id = $1", id)
		pool.Exec(ctx, "DELETE FROM campaigns WHERE id = $1", id)
		pool.Exec(ctx, "DELETE FROM transactions WHERE account_id = $1", accountID)
		pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
		funding, err := db.New(pool).GetFundingAccount(ctx, p.Currency)
		if err == nil {
			pool.Exec(ctx, "DELETE FROM transactions WHERE account_id = $1 AND description = $2", funding, p.Name)
			db.New(pool).SyncAccountBalance(ctx, funding)
		}
	})
	return id, accountID
}

func payAt(t *testing.T, pool *pgxpool.Pool, u testUser, amount int64, at time.Time) PayResult {
	t.Helper()
	r, err := Pay(context.Background(), pool, PayParams{
		UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: amount, MerchantName: "Kopi Senja",
		IdempotencyKey: fmt.Sprintf("cb-%d", time.Now().UnixNano()), Now: at,
	})
	if err != nil {
		t.Fatalf("pay %d: %v", amount, err)
	}
	return r
}

// assertLedger checks every double-entry invariant.
func assertLedger(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	if drift, err := q.ReconcileAccounts(ctx); err != nil || len(drift) != 0 {
		t.Errorf("account balances drifted from their transactions: %+v, %v", drift, err)
	}
	if bad, err := q.ReconcileAwards(ctx); err != nil || len(bad) != 0 {
		t.Errorf("awards whose two halves don't match: %v, %v", bad, err)
	}
	if n, err := q.CampaignFlowImbalance(ctx); err != nil || n != 0 {
		t.Errorf("campaign money doesn't net to zero: off by %d (%v)", n, err)
	}
}

func wantCashback(t *testing.T, r PayResult, amount int64, reason cashback.Reason) {
	t.Helper()
	if r.Cashback == nil {
		t.Fatalf("cashback = nil, want %d %s", amount, reason)
	}
	if r.Cashback.AmountMinor != amount || r.Cashback.Reason != reason {
		t.Errorf("cashback = %d %s, want %d %s", r.Cashback.AmountMinor, r.Cashback.Reason, amount, reason)
	}
}

var noon = time.Date(2026, 10, 13, 12, 0, 0, 0, jakarta)

func TestCashbackMinimumSpend(t *testing.T) {
	pool := openTestDB(t)
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)

	wantCashback(t, payAt(t, pool, u, 1_999_900, noon), 0, cashback.BelowMinimum)  // Rp 19.999
	wantCashback(t, payAt(t, pool, u, 2_000_000, noon), 100_000, cashback.Awarded) // Rp 20.000 → Rp 1.000
	assertLedger(t, pool)
}

func TestCashbackDailyCapFollowsJakartaMidnight(t *testing.T) {
	pool := openTestDB(t)
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 10_000_000_000)

	// Rp 400.000 earns Rp 20.000; the third payment only fits Rp 10.000 under the cap.
	wantCashback(t, payAt(t, pool, u, 40_000_000, noon), 2_000_000, cashback.Awarded)
	wantCashback(t, payAt(t, pool, u, 40_000_000, noon), 2_000_000, cashback.Awarded)
	wantCashback(t, payAt(t, pool, u, 40_000_000, noon), 1_000_000, cashback.Awarded)

	lastSecond := time.Date(2026, 10, 13, 23, 59, 59, 0, jakarta)
	wantCashback(t, payAt(t, pool, u, 40_000_000, lastSecond), 0, cashback.DailyCapReached)

	midnight := time.Date(2026, 10, 14, 0, 0, 0, 0, jakarta)
	wantCashback(t, payAt(t, pool, u, 40_000_000, midnight), 2_000_000, cashback.Awarded)

	// The day boundary is Jakarta's, not UTC's: midnight WIB on the 14th is
	// still 17:00 UTC on the 13th.
	var day time.Time
	pool.QueryRow(context.Background(), "SELECT max(campaign_day) FROM cashback_awards WHERE user_id = $1", u.id).Scan(&day)
	if got := day.Format(time.DateOnly); got != "2026-10-14" {
		t.Errorf("campaign_day = %s, want 2026-10-14", got)
	}
	assertLedger(t, pool)
}

func TestCashbackPaysWhatsLeftThenEnds(t *testing.T) {
	pool := openTestDB(t)
	id, account := launch(t, pool, launchParams(300_000)) // Rp 3.000
	u := payingUser(t, pool, 1_000_000_000)

	wantCashback(t, payAt(t, pool, u, 3_500_000, noon), 175_000, cashback.Awarded) // Rp 1.750, Rp 1.250 left
	wantCashback(t, payAt(t, pool, u, 5_000_000, noon), 125_000, cashback.Awarded) // due Rp 2.500, gets Rp 1.250

	if b := balanceOf(t, pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	var ended bool
	pool.QueryRow(context.Background(), "SELECT ends_at IS NOT NULL FROM campaigns WHERE id = $1", id).Scan(&ended)
	if !ended {
		t.Error("campaign still open after its budget ran out")
	}
	// No campaign is live any more.
	if r := payAt(t, pool, u, 5_000_000, noon.Add(time.Minute)); r.Cashback != nil {
		t.Errorf("cashback after the campaign ended = %+v, want nil", r.Cashback)
	}
	assertLedger(t, pool)
}

// Without a time of its own, a payment takes the database's clock, so API
// servers with skewed clocks still agree on the campaign day.
func TestCashbackDayComesFromDatabaseClock(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)

	var before, after time.Time
	pool.QueryRow(ctx, "SELECT now()").Scan(&before)
	r := payAt(t, pool, u, 3_500_000, time.Time{})
	pool.QueryRow(ctx, "SELECT now()").Scan(&after)

	wantCashback(t, r, 175_000, cashback.Awarded)
	if at := r.Transaction.CreatedAt; at.Before(before) || at.After(after) {
		t.Errorf("payment at %s, want between database times %s and %s", at, before, after)
	}
	var matches bool
	err := pool.QueryRow(ctx, `SELECT campaign_day = ($2::timestamptz AT TIME ZONE 'Asia/Jakarta')::date
		FROM cashback_awards WHERE payment_transaction_id = $1`, r.Transaction.ID, r.Transaction.CreatedAt).Scan(&matches)
	if err != nil || !matches {
		t.Errorf("campaign_day doesn't match the payment's database time (%v)", err)
	}
	assertLedger(t, pool)
}

// A user with no cash back account can still pay during a campaign. A
// payment that would have earned is reported unavailable, not failed.
func TestCashbackWithoutCashbackAccountStillPays(t *testing.T) {
	pool := openTestDB(t)
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)
	if _, err := pool.Exec(context.Background(), "DELETE FROM accounts WHERE id = $1", u.cashback); err != nil {
		t.Fatal(err)
	}

	wantCashback(t, payAt(t, pool, u, 1_999_900, noon), 0, cashback.BelowMinimum)
	wantCashback(t, payAt(t, pool, u, 3_500_000, noon), 0, cashback.Unavailable)
	if b := balanceOf(t, pool, u.main); b != 1_000_000_000-1_999_900-3_500_000 {
		t.Errorf("main balance = %d, want both payments debited", b)
	}
	assertLedger(t, pool)
}

// Twenty users race for a Rp 10.000 budget. The awards add up to exactly
// the budget, and it never goes negative.
func TestCashbackConcurrentPaymentsNeverOverspendBudget(t *testing.T) {
	pool := openTestDB(t)
	_, account := launch(t, pool, launchParams(1_000_000))

	users := make([]testUser, 20)
	for i := range users {
		users[i] = payingUser(t, pool, 1_000_000_000)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	for _, u := range users {
		wg.Go(func() {
			r, err := Pay(context.Background(), pool, PayParams{
				UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000,
				MerchantName: "Kopi Senja", IdempotencyKey: "race-" + u.id, Now: noon,
			})
			if err != nil {
				t.Errorf("pay: %v", err)
				return
			}
			if r.Cashback != nil {
				mu.Lock()
				total += r.Cashback.AmountMinor
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if total != 1_000_000 {
		t.Errorf("total awarded = %d, want exactly the budget 1000000", total)
	}
	if b := balanceOf(t, pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	assertLedger(t, pool)
}

func TestCashbackReplayDoesNotAwardTwice(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)

	p := PayParams{UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000,
		MerchantName: "Kopi Senja", IdempotencyKey: "replay-1", Now: noon}
	first, err := Pay(ctx, pool, p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Pay(ctx, pool, p)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed {
		t.Error("second payment wasn't a replay")
	}
	wantCashback(t, first, 175_000, cashback.Awarded)
	wantCashback(t, second, 175_000, cashback.Awarded)
	if b := balanceOf(t, pool, u.cashback); b != 175_000 {
		t.Errorf("cash back balance = %d, want one award", b)
	}
	assertLedger(t, pool)
}

func TestCashbackOnlyWhileACampaignIsLive(t *testing.T) {
	pool := openTestDB(t)
	u := payingUser(t, pool, 1_000_000_000)

	t.Run("none", func(t *testing.T) {
		if r := payAt(t, pool, u, 3_500_000, noon); r.Cashback != nil {
			t.Errorf("cashback = %+v, want nil", r.Cashback)
		}
	})
	t.Run("not started yet", func(t *testing.T) {
		p := launchParams(1_000_000_000)
		p.StartsAt = noon.Add(time.Hour)
		launch(t, pool, p)
		if r := payAt(t, pool, u, 3_500_000, noon); r.Cashback != nil {
			t.Errorf("cashback = %+v, want nil", r.Cashback)
		}
	})
	t.Run("other currency", func(t *testing.T) {
		p := launchParams(1_000_000)
		p.Currency = "USD"
		launch(t, pool, p)
		if r := payAt(t, pool, u, 3_500_000, noon); r.Cashback != nil {
			t.Errorf("cashback = %+v, want nil", r.Cashback)
		}
	})
	assertLedger(t, pool)
}

func TestCampaignsCannotOverlap(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	first := launchParams(1_000_000)
	end := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	first.EndsAt = &end
	launch(t, pool, first)

	overlapping := launchParams(1_000_000)
	overlapping.StartsAt = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, err := LaunchCampaign(ctx, pool, overlapping); !errors.Is(err, ErrCampaignOverlap) {
		t.Errorf("overlapping launch = %v, want ErrCampaignOverlap", err)
	}

	after := launchParams(1_000_000)
	after.StartsAt = end
	launch(t, pool, after) // back to back is fine

	bad := launchParams(1_000_000)
	bad.StartsAt = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	bad.Timezone = "Mars/Olympus"
	if _, err := LaunchCampaign(ctx, pool, bad); !errors.Is(err, ErrBadTimezone) {
		t.Errorf("bad time zone = %v, want ErrBadTimezone", err)
	}
	assertLedger(t, pool)
}

func TestEndCampaignReturnsUnspentBudget(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	p := launchParams(1_000_000)
	p.StartsAt = time.Now().Add(-time.Hour)
	id, account := launch(t, pool, p)
	u := payingUser(t, pool, 1_000_000_000)

	wantCashback(t, payAt(t, pool, u, 3_500_000, time.Now()), 175_000, cashback.Awarded)
	returned, err := EndCampaign(ctx, pool, nil, id)
	if err != nil {
		t.Fatal(err)
	}
	if returned != 825_000 {
		t.Errorf("returned %d, want 825000", returned)
	}
	if b := balanceOf(t, pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	if r := payAt(t, pool, u, 3_500_000, time.Now()); r.Cashback != nil {
		t.Errorf("cashback after end = %+v, want nil", r.Cashback)
	}
	assertLedger(t, pool)
}

func TestEarnedCashbackIsListed(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)
	other := payingUser(t, pool, 1_000_000_000)

	earned := payAt(t, pool, u, 3_500_000, noon)
	payAt(t, pool, u, 1_000_000, noon)     // below the minimum
	payAt(t, pool, other, 3_500_000, noon) // someone else's

	q := db.New(pool)
	rows, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: u.id, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		switch {
		case r.ID == earned.Transaction.ID:
			if r.CashbackMinor == nil || *r.CashbackMinor != 175_000 {
				t.Errorf("earning payment cashback = %v, want 175000", r.CashbackMinor)
			}
		case r.CashbackMinor != nil:
			t.Errorf("%s %d shows cashback %d, want none", r.Name, r.AmountMinor, *r.CashbackMinor)
		}
	}

	history, err := q.ListCashbackTransactions(ctx, db.ListCashbackTransactionsParams{UserID: u.id, Currency: "IDR", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].AmountMinor != 175_000 || history[0].Description != "From Kopi Senja" {
		t.Errorf("history = %+v, want one +175000 'From Kopi Senja' row", history)
	}
}

// One user's concurrent payments share a daily cap: each award must see the
// awards committed before it, or the user would earn past the cap.
func TestCashbackConcurrentPaymentsKeepDailyCap(t *testing.T) {
	pool := openTestDB(t)
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	for i := range 20 {
		wg.Go(func() {
			// Rp 100.000 earns Rp 5.000: 10 of these reach the Rp 50.000 cap.
			r, err := Pay(context.Background(), pool, PayParams{
				UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 10_000_000,
				MerchantName: "Kopi Senja", IdempotencyKey: fmt.Sprintf("cap-%d", i), Now: noon,
			})
			if err != nil {
				t.Errorf("pay %d: %v", i, err)
				return
			}
			mu.Lock()
			total += r.Cashback.AmountMinor
			mu.Unlock()
		})
	}
	wg.Wait()

	if total != 5_000_000 {
		t.Errorf("total awarded = %d, want exactly the daily cap 5000000", total)
	}
	if b := balanceOf(t, pool, u.cashback); b != 5_000_000 {
		t.Errorf("cash back balance = %d, want 5000000", b)
	}
	assertLedger(t, pool)
}

// Ending a campaign while payments are being awarded neither deadlocks nor
// returns more budget than is left.
func TestEndCampaignDuringPayments(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	id, account := launch(t, pool, launchParams(1_000_000_000))

	users := make([]testUser, 10)
	for i := range users {
		users[i] = payingUser(t, pool, 1_000_000_000)
	}
	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			for i := range 5 {
				if _, err := Pay(ctx, pool, PayParams{
					UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000,
					MerchantName: "Kopi Senja", IdempotencyKey: fmt.Sprintf("end-%s-%d", u.id, i),
				}); err != nil {
					t.Errorf("pay: %v", err)
				}
			}
		})
	}
	wg.Go(func() {
		time.Sleep(5 * time.Millisecond)
		if _, err := EndCampaign(ctx, pool, nil, id); err != nil {
			t.Errorf("end campaign: %v", err)
		}
	})
	wg.Wait()

	if b := balanceOf(t, pool, account); b != 0 {
		t.Errorf("campaign account = %d after ending, want 0", b)
	}
	assertLedger(t, pool)
}

// An awarding payment in db mode is seven round trips: BEGIN, the debit,
// the payment row, finding the campaign and its gate (no lock), the
// campaign lock, the award, COMMIT. Every awarding payment queues on the
// campaign lock, so a statement added from the lock on costs throughput.
func TestPayRoundTrips(t *testing.T) {
	pool := openTestDB(t)
	launch(t, pool, launchParams(1_000_000_000))
	u := payingUser(t, pool, 1_000_000_000)

	ctx, stats := dbtrace.WithStats(context.Background())
	r, err := Pay(ctx, pool, PayParams{
		UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000,
		MerchantName: "Kopi Senja", IdempotencyKey: fmt.Sprintf("trips-%d", time.Now().UnixNano()), Now: noon,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCashback(t, r, 175_000, cashback.Awarded)
	if n := stats.Queries(); n != 7 {
		t.Errorf("awarding payment ran %d queries, want 7", n)
	}
	if stats.QueryTime() <= 0 || stats.WaitTime() <= 0 {
		t.Errorf("stats = %v query, %v wait; want both measured", stats.QueryTime(), stats.WaitTime())
	}
}
