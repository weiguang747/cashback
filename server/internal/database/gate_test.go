package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cashback/server/internal/budgetgate"
	"cashback/server/internal/cashback"
	"cashback/server/internal/db"
	"cashback/server/internal/dbtrace"
)

// These need TEST_REDIS_URL as well as TEST_DATABASE_URL.

func openTestRedis(t *testing.T) (*budgetgate.Redis, *redis.Client) {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { client.Close() })
	return budgetgate.NewRedis(client), client
}

type gateEnv struct {
	pool   *pgxpool.Pool
	redis  *budgetgate.Redis
	client *redis.Client
	gate   *CashbackGate
	rec    *Reconciler
}

func newGateEnv(t *testing.T) gateEnv {
	t.Helper()
	pool := openTestDB(t)
	r, client := openTestRedis(t)
	g := NewCashbackGate(pool, r)
	t.Cleanup(g.Wait)
	return gateEnv{pool: pool, redis: r, client: client, gate: g, rec: &Reconciler{Pool: pool, Gate: r}}
}

// launchGated launches a campaign and switches it to gate, removing its
// Redis keys when the test ends.
func (e gateEnv) launchGated(t *testing.T, budget int64, gate string) (id, account string) {
	t.Helper()
	id, account = launch(t, e.pool, launchParams(budget))
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := e.client.Keys(ctx, "campaign:{"+id+"}:*").Result()
		if len(keys) > 0 {
			e.client.Del(ctx, keys...)
		}
	})
	if err := e.rec.SetGate(context.Background(), id, gate); err != nil {
		t.Fatalf("set gate %s: %v", gate, err)
	}
	return id, account
}

func (e gateEnv) pay(t *testing.T, u testUser, amount int64) PayResult {
	t.Helper()
	r, err := e.payErr(context.Background(), u, amount, e.gate)
	if err != nil {
		t.Fatalf("pay %d: %v", amount, err)
	}
	return r
}

// payKeys makes idempotency keys unique even for payments started in the
// same clock tick (macOS clocks tick in microseconds); a repeated key would
// replay the earlier payment instead of paying.
var payKeys atomic.Int64

func (e gateEnv) payErr(ctx context.Context, u testUser, amount int64, g *CashbackGate) (PayResult, error) {
	return Pay(ctx, e.pool, PayParams{
		UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: amount, MerchantName: "Kopi Senja",
		IdempotencyKey: fmt.Sprintf("gate-%d-%d", time.Now().UnixNano(), payKeys.Add(1)), Now: noon, Gate: g,
	})
}

// creditAll runs the award worker until the queue is empty.
func creditAll(t *testing.T, pool *pgxpool.Pool) CreditResult {
	t.Helper()
	var total CreditResult
	for {
		r, err := CreditPendingAwards(context.Background(), pool, AwardBatch)
		if err != nil {
			t.Fatalf("credit pending awards: %v", err)
		}
		total.Credited += r.Credited
		total.Failed += r.Failed
		total.Ended = append(total.Ended, r.Ended...)
		if r.Credited+r.Failed == 0 {
			return total
		}
	}
}

func pendingFor(t *testing.T, pool *pgxpool.Pool, campaign string) int64 {
	t.Helper()
	n, err := db.New(pool).PendingForCampaign(context.Background(), campaign)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e gateEnv) counter(t *testing.T, campaign string) int64 {
	t.Helper()
	s, err := e.redis.Snapshot(context.Background(), campaign)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Open {
		t.Fatal("counter is closed")
	}
	return s.Remaining
}

// assertNoDrift checks the counter matches the database exactly.
func (e gateEnv) assertNoDrift(t *testing.T, campaign string) {
	t.Helper()
	e.gate.Wait()
	res, err := e.rec.Reconcile(context.Background(), campaign, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Drift != 0 {
		t.Errorf("counter drifted from the database by %d", res.Drift)
	}
}

func TestGateAwardIsPendingThenCredited(t *testing.T) {
	e := newGateEnv(t)
	id, account := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)

	r := e.pay(t, u, 3_500_000)
	wantCashback(t, r, 175_000, cashback.Awarded)
	if !r.Cashback.Pending {
		t.Error("award not reported as pending")
	}
	if b := balanceOf(t, e.pool, u.cashback); b != 0 {
		t.Errorf("cash back balance = %d before the worker ran, want 0", b)
	}
	if p := pendingFor(t, e.pool, id); p != 175_000 {
		t.Errorf("pending = %d, want 175000", p)
	}
	if c := e.counter(t, id); c != 1_000_000_000-175_000 {
		t.Errorf("counter = %d, want the budget less the award", c)
	}

	if got := creditAll(t, e.pool); got.Credited != 1 {
		t.Fatalf("worker credited %d, want 1", got.Credited)
	}
	if b := balanceOf(t, e.pool, u.cashback); b != 175_000 {
		t.Errorf("cash back balance = %d after the worker, want 175000", b)
	}
	if b := balanceOf(t, e.pool, account); b != 1_000_000_000-175_000 {
		t.Errorf("campaign account = %d", b)
	}
	if p := pendingFor(t, e.pool, id); p != 0 {
		t.Errorf("pending = %d after crediting, want 0", p)
	}
	if got := creditAll(t, e.pool); got.Credited != 0 {
		t.Errorf("second run credited %d, want 0", got.Credited)
	}
	e.assertNoDrift(t, id)
	assertLedger(t, e.pool)
}

// What the app shows as earned today includes awards still pending, as the
// daily cap does, both before and after the worker credits them.
func TestGateEarnedTodayCountsPendingAwards(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)

	// Paid now (no fixed time), since the status is for the database's today.
	r, err := Pay(ctx, e.pool, PayParams{
		UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000, MerchantName: "Kopi Senja",
		IdempotencyKey: fmt.Sprintf("gate-%d-%d", time.Now().UnixNano(), payKeys.Add(1)), Gate: e.gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Cashback == nil || !r.Cashback.Pending {
		t.Fatalf("cashback = %+v, want a pending award", r.Cashback)
	}

	earnedToday := func() int64 {
		t.Helper()
		st, err := db.New(e.pool).GetCampaignStatus(ctx, db.GetCampaignStatusParams{UserID: u.id, Currency: "IDR"})
		if err != nil {
			t.Fatal(err)
		}
		return st.EarnedTodayMinor
	}
	if got := earnedToday(); got != 175_000 {
		t.Errorf("earned today while pending = %d, want 175000", got)
	}
	creditAll(t, e.pool)
	if got := earnedToday(); got != 175_000 {
		t.Errorf("earned today once credited = %d, want 175000", got)
	}
}

// Twenty users race for a Rp 10.000 budget through the gate. The grants add
// up to exactly the budget, and once credited so do the awards; the
// campaign ends at zero with nothing failed.
func TestGateConcurrentPaymentsNeverOverspend(t *testing.T) {
	e := newGateEnv(t)
	id, account := e.launchGated(t, 1_000_000, GateRedis)
	users := make([]testUser, 20)
	for i := range users {
		users[i] = payingUser(t, e.pool, 1_000_000_000)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var granted int64
	for _, u := range users {
		wg.Go(func() {
			r, err := e.payErr(context.Background(), u, 3_500_000, e.gate)
			if err != nil {
				t.Errorf("pay: %v", err)
				return
			}
			if r.Cashback != nil {
				mu.Lock()
				granted += r.Cashback.AmountMinor
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if granted != 1_000_000 {
		t.Errorf("granted %d in total, want exactly the budget 1000000", granted)
	}
	got := creditAll(t, e.pool)
	if got.Failed != 0 {
		t.Errorf("%d awards failed", got.Failed)
	}
	if len(got.Ended) != 1 || got.Ended[0] != id {
		t.Errorf("campaigns ended = %v, want %s once", got.Ended, id)
	}
	if b := balanceOf(t, e.pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	var awarded int64
	if err := e.pool.QueryRow(context.Background(),
		"SELECT COALESCE(SUM(amount_minor), 0) FROM cashback_awards WHERE campaign_id = $1", id).Scan(&awarded); err != nil {
		t.Fatal(err)
	}
	if awarded != 1_000_000 {
		t.Errorf("credited %d, want 1000000", awarded)
	}
	assertLedger(t, e.pool)
}

// One user's concurrent payments can't earn past the daily cap, whether
// earlier awards are credited or still pending.
// Under the Redis gate too, a user with no cash back account pays without
// cash back and reserves nothing.
func TestGateWithoutCashbackAccountStillPays(t *testing.T) {
	e := newGateEnv(t)
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	if _, err := e.pool.Exec(context.Background(), "DELETE FROM accounts WHERE id = $1", u.cashback); err != nil {
		t.Fatal(err)
	}

	wantCashback(t, e.pay(t, u, 3_500_000), 0, cashback.Unavailable)
	if p := pendingFor(t, e.pool, id); p != 0 {
		t.Errorf("pending = %d, want 0", p)
	}
	e.assertNoDrift(t, id)
	assertLedger(t, e.pool)
}

func TestGateDailyCapCountsPendingAwards(t *testing.T) {
	e := newGateEnv(t)
	e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 10_000_000_000)

	// Each earns Rp 17.500; the cap is Rp 50.000.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var earned int64
	for i := range 10 {
		wg.Go(func() {
			if i == 5 { // some credited, some pending
				if _, err := CreditPendingAwards(context.Background(), e.pool, AwardBatch); err != nil {
					t.Error(err)
				}
			}
			r, err := e.payErr(context.Background(), u, 35_000_000, e.gate)
			if err != nil {
				t.Errorf("pay: %v", err)
				return
			}
			mu.Lock()
			earned += r.Cashback.AmountMinor
			mu.Unlock()
		})
	}
	wg.Wait()
	creditAll(t, e.pool)

	if earned != 5_000_000 {
		t.Errorf("earned %d, want exactly the daily cap 5000000", earned)
	}
	if b := balanceOf(t, e.pool, u.cashback); b != 5_000_000 {
		t.Errorf("cash back balance = %d, want 5000000", b)
	}
	assertLedger(t, e.pool)
}

// failingGate fails every Reserve, after delay if set.
type failingGate struct{ delay time.Duration }

func (f failingGate) Reserve(ctx context.Context, _, _ string, _, _ int64) (budgetgate.Grant, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return budgetgate.Grant{}, ctx.Err()
		}
	}
	return budgetgate.Grant{}, errors.New("redis: connection refused")
}

func (failingGate) Release(context.Context, string, string) error { return nil }

// When the gate fails, the payment still goes through, without cash back;
// enough failures degrade the campaign.
func TestGateDownFailsClosedThenDegrades(t *testing.T) {
	e := newGateEnv(t)
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	down := NewCashbackGate(e.pool, failingGate{})

	for i := range degradeAfter {
		r, err := e.payErr(context.Background(), u, 3_500_000, down)
		if err != nil {
			t.Fatalf("payment %d failed with the gate down: %v", i, err)
		}
		wantCashback(t, r, 0, cashback.Unavailable)
	}
	down.Wait()
	if p := pendingFor(t, e.pool, id); p != 0 {
		t.Errorf("pending = %d, want nothing reserved", p)
	}
	if b := balanceOf(t, e.pool, u.main); b != 1_000_000_000-degradeAfter*3_500_000 {
		t.Errorf("main balance = %d, want every payment charged", b)
	}
	var degraded bool
	if err := e.pool.QueryRow(context.Background(),
		"SELECT gate_degraded_at IS NOT NULL FROM campaigns WHERE id = $1", id).Scan(&degraded); err != nil {
		t.Fatal(err)
	}
	if !degraded {
		t.Error("campaign not degraded after repeated gate failures")
	}
	assertLedger(t, e.pool)
}

func TestGateTimeoutFailsClosed(t *testing.T) {
	e := newGateEnv(t)
	e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	slow := NewCashbackGate(e.pool, failingGate{delay: time.Second})
	slow.ReserveTimeout = 20 * time.Millisecond

	start := time.Now()
	r, err := e.payErr(context.Background(), u, 3_500_000, slow)
	if err != nil {
		t.Fatal(err)
	}
	wantCashback(t, r, 0, cashback.Unavailable)
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("payment took %v; the reserve timeout should bound it", d)
	}
}

// degrade marks the campaign degraded, settled long enough ago (or just now).
func degrade(t *testing.T, pool *pgxpool.Pool, id string, ago time.Duration) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE campaigns SET gate_degraded_at = now() - $2::int * interval '1 second' WHERE id = $1",
		id, int(ago/time.Second)); err != nil {
		t.Fatal(err)
	}
}

// A degraded campaign gives nothing while earlier reservations settle, then
// awards under the lock, exactly, until the gate recovers.
func TestGateDegradedFallsBackToLockThenRecovers(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, account := e.launchGated(t, 1_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)

	// One award pending from before the outage.
	wantCashback(t, e.pay(t, u, 3_500_000), 175_000, cashback.Awarded)

	degrade(t, e.pool, id, 0)
	wantCashback(t, e.pay(t, u, 3_500_000), 0, cashback.Unavailable)
	if err := e.rec.Recover(ctx, id); !errors.Is(err, ErrGateNotReady) {
		t.Fatalf("Recover right after degrading = %v, want ErrGateNotReady", err)
	}

	degrade(t, e.pool, id, GateSettleTime+time.Second)
	r := e.pay(t, u, 3_500_000)
	wantCashback(t, r, 175_000, cashback.Awarded)
	if r.Cashback.Pending {
		t.Error("fallback award reported pending; it is credited under the lock")
	}

	// Concurrent fallback payments race for what's left, counting the
	// pending award: 1,000,000 - 2 x 175,000 = 650,000.
	users := make([]testUser, 10)
	for i := range users {
		users[i] = payingUser(t, e.pool, 1_000_000_000)
	}
	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			if _, err := e.payErr(ctx, u, 3_500_000, e.gate); err != nil {
				t.Errorf("pay: %v", err)
			}
		})
	}
	wg.Wait()
	if b, p := balanceOf(t, e.pool, account), pendingFor(t, e.pool, id); b != p {
		t.Errorf("campaign account %d, pending %d: the fallback should spend everything else", b, p)
	}

	if err := e.rec.Recover(ctx, id); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if c := e.counter(t, id); c != 0 {
		t.Errorf("counter after recovery = %d, want 0 (all spent or pending)", c)
	}
	wantCashback(t, e.pay(t, u, 3_500_000), 0, cashback.BudgetExhausted)

	if got := creditAll(t, e.pool); got.Failed != 0 || got.Credited != 1 {
		t.Errorf("worker = %+v, want the one pending award credited", got)
	}
	if b := balanceOf(t, e.pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	e.assertNoDrift(t, id)
	assertLedger(t, e.pool)
}

// cancelAfterReserve reserves for real, then cancels the payment, so its
// transaction fails after the reservation.
type cancelAfterReserve struct {
	budgetgate.Gate
	cancel context.CancelFunc
}

func (c cancelAfterReserve) Reserve(ctx context.Context, campaign, payment string, want, inc int64) (budgetgate.Grant, error) {
	g, err := c.Gate.Reserve(ctx, campaign, payment, want, inc)
	c.cancel()
	return g, err
}

func TestGateReleasesWhenPaymentFails(t *testing.T) {
	e := newGateEnv(t)
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)

	ctx, cancel := context.WithCancel(context.Background())
	g := NewCashbackGate(e.pool, cancelAfterReserve{Gate: e.redis, cancel: cancel})
	if _, err := e.payErr(ctx, u, 3_500_000, g); err == nil {
		t.Fatal("payment succeeded; want it to fail after reserving")
	}
	g.Wait()

	if c := e.counter(t, id); c != 1_000_000_000 {
		t.Errorf("counter = %d, want the reservation returned", c)
	}
	if b := balanceOf(t, e.pool, u.main); b != 1_000_000_000 {
		t.Errorf("main balance = %d, want the payment rolled back", b)
	}
	e.assertNoDrift(t, id)
}

// A reservation whose payment never reached the database is released once
// it's older than GateSettleTime, exactly once; younger ones are left alone.
func TestReconcilerReleasesLeakedReservationOnce(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, _ := e.launchGated(t, 1_000_000, GateRedis)

	// Payment ids that never reached the database.
	const leaked, young = "00000000-0000-4000-8000-00000000000a", "00000000-0000-4000-8000-00000000000b"
	for _, p := range []string{leaked, young} {
		if g, err := e.redis.Reserve(ctx, id, p, 100_000, 100); err != nil || g.Outcome != budgetgate.Granted {
			t.Fatalf("reserve %s: %+v, %v", p, g, err)
		}
	}
	old := time.Now().Add(-GateSettleTime - time.Minute).Unix()
	e.client.HSet(ctx, "campaign:{"+id+"}:inflight", leaked, fmt.Sprintf("100000:%d", old))

	res, err := e.rec.Reconcile(ctx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Drift != 0 || res.Released != 1 || res.InFlight != 1 {
		t.Errorf("pass = %+v, want no drift, 1 released, 1 still in flight", res)
	}
	if c := e.counter(t, id); c != 900_000 {
		t.Errorf("counter = %d, want 900000 (the young reservation still held)", c)
	}
	res, err = e.rec.Reconcile(ctx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Released != 0 || res.Drift != 0 {
		t.Errorf("second pass = %+v, want nothing more released", res)
	}
	if c := e.counter(t, id); c != 900_000 {
		t.Errorf("counter = %d after a second pass, want 900000", c)
	}
}

func TestReconcilerCorrectsDrift(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	e.pay(t, u, 3_500_000) // pending
	e.pay(t, u, 3_500_000)
	creditAll(t, e.pool)
	e.pay(t, u, 3_500_000) // pending
	want := int64(1_000_000_000 - 3*175_000)

	for _, drift := range []int64{250_000, -40_000_000} {
		e.client.IncrBy(ctx, "campaign:{"+id+"}:remaining", drift)
		res, err := e.rec.Reconcile(ctx, id, true)
		if err != nil {
			t.Fatal(err)
		}
		if res.Drift != -drift || !res.Corrected {
			t.Errorf("pass after drifting %d = %+v, want drift %d corrected", drift, res, -drift)
		}
		if c := e.counter(t, id); c != want {
			t.Errorf("counter = %d, want %d", c, want)
		}
	}
	if res, _ := e.rec.Reconcile(ctx, id, true); res.Drift != 0 || res.Pruned != 0 {
		t.Errorf("third pass = %+v, want nothing to do", res)
	}
}

// If Redis loses a campaign's keys (restarted without its data), the
// reconciler degrades the campaign, so it awards under the lock, and then
// reseeds the counter from the database.
func TestReconcilerRecoversLostCounter(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	e.pay(t, u, 3_500_000) // pending
	want := int64(1_000_000_000 - 175_000)

	e.client.Del(ctx, "campaign:{"+id+"}:remaining", "campaign:{"+id+"}:inflight")
	wantCashback(t, e.pay(t, u, 3_500_000), 0, cashback.Unavailable)

	row := db.ListGatedCampaignsRow{ID: id, BudgetGate: GateRedis}
	log := slog.New(slog.DiscardHandler)
	e.rec.reconcileOne(ctx, log, row)
	var degraded bool
	if err := e.pool.QueryRow(ctx,
		"SELECT gate_degraded_at IS NOT NULL FROM campaigns WHERE id = $1", id).Scan(&degraded); err != nil {
		t.Fatal(err)
	}
	if !degraded {
		t.Fatal("campaign not degraded after its counter disappeared")
	}

	degrade(t, e.pool, id, GateSettleTime+time.Second)
	row.GateDegraded = true
	e.rec.reconcileOne(ctx, log, row)
	if c := e.counter(t, id); c != want {
		t.Errorf("counter after recovery = %d, want %d", c, want)
	}
	wantCashback(t, e.pay(t, u, 3_500_000), 175_000, cashback.Awarded)
	e.assertNoDrift(t, id)
}

// A campaign switched to db while the reconciler still had it listed as
// redis isn't degraded: its counter is closed on purpose.
func TestReconcilerIgnoresCounterClosedBySwitch(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	if err := e.rec.SetGate(ctx, id, GateDB); err != nil {
		t.Fatal(err)
	}
	e.rec.reconcileOne(ctx, slog.New(slog.DiscardHandler), db.ListGatedCampaignsRow{ID: id, BudgetGate: GateRedis})
	var degraded bool
	if err := e.pool.QueryRow(ctx,
		"SELECT gate_degraded_at IS NOT NULL FROM campaigns WHERE id = $1", id).Scan(&degraded); err != nil {
		t.Fatal(err)
	}
	if degraded {
		t.Error("campaign degraded after switching to db")
	}
}

// Reconciling while payments reserve and the worker credits finds no drift:
// the two-snapshot arithmetic is exact, not approximate.
func TestReconcilerExactUnderLoad(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	users := make([]testUser, 8)
	for i := range users {
		users[i] = payingUser(t, e.pool, 100_000_000_000)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := e.payErr(ctx, u, 2_000_000, e.gate); err != nil {
					t.Errorf("pay: %v", err)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := CreditPendingAwards(ctx, e.pool, 10); err != nil {
				t.Errorf("credit: %v", err)
				return
			}
		}
	})
	for range 20 {
		res, err := e.rec.Reconcile(ctx, id, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Drift != 0 {
			t.Errorf("drift %d under load: %+v", res.Drift, res)
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	creditAll(t, e.pool)
	e.assertNoDrift(t, id)
	assertLedger(t, e.pool)
}

// A worker that dies mid-batch credits nothing; the next run credits each
// award exactly once.
func TestWorkerCrashDoesNotCreditTwice(t *testing.T) {
	e := newGateEnv(t)
	id, _ := e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	for range 3 {
		e.pay(t, u, 3_500_000)
	}

	dead, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CreditPendingAwards(dead, e.pool, AwardBatch); err == nil {
		t.Fatal("worker with a canceled context succeeded")
	}
	if p := pendingFor(t, e.pool, id); p != 3*175_000 {
		t.Fatalf("pending = %d after the crash, want all three still pending", p)
	}
	creditAll(t, e.pool)
	creditAll(t, e.pool)
	if b := balanceOf(t, e.pool, u.cashback); b != 3*175_000 {
		t.Errorf("cash back balance = %d, want each award credited once", b)
	}
	assertLedger(t, e.pool)
}

// An award the campaign account can't cover is marked failed, never
// retried, and still counted against the budget.
func TestWorkerMarksUncoverableAwardFailed(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, account := e.launchGated(t, 1_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	r := e.pay(t, u, 3_500_000)

	// Make the reservation bigger than the whole budget, as if Redis had
	// been seeded wrong.
	if _, err := e.pool.Exec(ctx, "UPDATE cashback_pending SET amount_minor = 2000000 WHERE payment_transaction_id = $1",
		r.Transaction.ID); err != nil {
		t.Fatal(err)
	}
	got := creditAll(t, e.pool)
	if got.Failed != 1 || got.Credited != 0 {
		t.Fatalf("worker = %+v, want one failed", got)
	}
	if b := balanceOf(t, e.pool, account); b != 1_000_000 {
		t.Errorf("campaign account = %d, want untouched", b)
	}
	if got := creditAll(t, e.pool); got.Failed+got.Credited != 0 {
		t.Errorf("failed award retried: %+v", got)
	}
	if n, _ := db.New(e.pool).CountFailedPendingAwards(ctx); n < 1 {
		t.Error("failed award not counted")
	}
	if age, _ := db.New(e.pool).OldestPendingAgeSecs(ctx); age != 0 {
		t.Errorf("backlog age = %d; failed awards shouldn't count as waiting", age)
	}
	if p := pendingFor(t, e.pool, id); p != 2_000_000 {
		t.Errorf("pending = %d, want the failed award still held", p)
	}
	assertLedger(t, e.pool)
}

// db -> shadow -> redis -> db, with awards along the way: every switch
// keeps the counter and the database in step, and switching back to db
// while awards are pending doesn't let the lock path spend them.
func TestSetGateRoundTrip(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, account := e.launchGated(t, 1_000_000, GateDB)
	u := payingUser(t, e.pool, 1_000_000_000)

	wantCashback(t, e.pay(t, u, 3_500_000), 175_000, cashback.Awarded) // db

	if err := e.rec.SetGate(ctx, id, GateShadow); err != nil {
		t.Fatal(err)
	}
	r := e.pay(t, u, 3_500_000)
	wantCashback(t, r, 175_000, cashback.Awarded)
	if r.Cashback.Pending {
		t.Error("shadow award reported pending")
	}
	e.gate.Wait()
	if c := e.counter(t, id); c != 1_000_000-2*175_000 {
		t.Errorf("shadow counter = %d, want it to mirror the database", c)
	}
	e.assertNoDrift(t, id)

	if err := e.rec.SetGate(ctx, id, GateRedis); err != nil {
		t.Fatal(err)
	}
	if r := e.pay(t, u, 3_500_000); !r.Cashback.Pending {
		t.Error("redis award not pending")
	}

	if err := e.rec.SetGate(ctx, id, GateDB); err != nil {
		t.Fatal(err)
	}
	// 1,000,000 - 3 x 175,000 = 475,000 left, 175,000 of it already pending
	// in the account: two more 175,000 awards, then 125,000.
	other := payingUser(t, e.pool, 10_000_000_000)
	wantCashback(t, e.pay(t, other, 3_500_000), 175_000, cashback.Awarded)
	wantCashback(t, e.pay(t, other, 3_500_000), 175_000, cashback.Awarded)
	wantCashback(t, e.pay(t, other, 3_500_000), 125_000, cashback.Awarded)
	wantCashback(t, e.pay(t, other, 3_500_000), 0, cashback.BudgetExhausted)
	if got := creditAll(t, e.pool); got.Credited != 1 || got.Failed != 0 {
		t.Errorf("worker = %+v, want the pending award credited", got)
	}
	if b := balanceOf(t, e.pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	assertLedger(t, e.pool)
}

// Ending a gated campaign returns only what isn't owed to pending awards;
// the worker still credits those.
func TestEndCampaignWithPendingAwards(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, account := e.launchGated(t, 1_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	e.pay(t, u, 3_500_000)
	e.pay(t, u, 3_500_000)

	if _, err := EndCampaign(ctx, e.pool, nil, id); !errors.Is(err, ErrGateNeeded) {
		t.Fatalf("EndCampaign without Redis = %v, want ErrGateNeeded", err)
	}
	returned, err := EndCampaign(ctx, e.pool, e.rec, id)
	if err != nil {
		t.Fatal(err)
	}
	if returned != 1_000_000-2*175_000 {
		t.Errorf("returned %d, want what isn't pending (650000)", returned)
	}
	if got := creditAll(t, e.pool); got.Credited != 2 || got.Failed != 0 {
		t.Errorf("worker = %+v, want both pending awards credited", got)
	}
	if b := balanceOf(t, e.pool, account); b != 0 {
		t.Errorf("campaign account = %d, want 0", b)
	}
	assertLedger(t, e.pool)
}

// Switching gates while payments run never spends more than the budget.
func TestGateSwitchDuringPayments(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, account := e.launchGated(t, 3_000_000, GateDB)
	users := make([]testUser, 10)
	for i := range users {
		users[i] = payingUser(t, e.pool, 1_000_000_000)
	}

	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			for range 4 {
				if _, err := e.payErr(ctx, u, 3_500_000, e.gate); err != nil {
					t.Errorf("pay: %v", err)
				}
			}
		})
	}
	wg.Go(func() {
		for _, gate := range []string{GateRedis, GateDB, GateShadow, GateRedis} {
			time.Sleep(2 * time.Millisecond)
			if err := e.rec.SetGate(ctx, id, gate); err != nil && !errors.Is(err, ErrCampaignOver) {
				t.Errorf("switch to %s: %v", gate, err)
			}
		}
	})
	wg.Wait()

	got := creditAll(t, e.pool)
	if got.Failed != 0 {
		t.Errorf("%d awards failed: the gates spent the same budget twice", got.Failed)
	}
	if b := balanceOf(t, e.pool, account); b < 0 {
		t.Errorf("campaign account = %d", b)
	}
	assertLedger(t, e.pool)
}

// A retried payment whose Redis idempotency key was lost reports its award,
// pending or credited.
func TestReplayReportsPendingAward(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	p := PayParams{UserID: u.id, AccountID: u.main, Currency: "IDR", AmountMinor: 3_500_000,
		MerchantName: "Kopi Senja", IdempotencyKey: fmt.Sprintf("replay-%d", time.Now().UnixNano()), Now: noon, Gate: e.gate}
	if _, err := Pay(ctx, e.pool, p); err != nil {
		t.Fatal(err)
	}

	again, err := Pay(ctx, e.pool, p)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.Cashback == nil || again.Cashback.AmountMinor != 175_000 || !again.Cashback.Pending {
		t.Errorf("replay = %+v / %+v, want the pending award", again, again.Cashback)
	}
	creditAll(t, e.pool)
	again, err = Pay(ctx, e.pool, p)
	if err != nil {
		t.Fatal(err)
	}
	if again.Cashback == nil || again.Cashback.AmountMinor != 175_000 || again.Cashback.Pending {
		t.Errorf("replay after crediting = %+v, want the credited award", again.Cashback)
	}
	assertLedger(t, e.pool)
}

// Pending cash back shows with the account but can't be redeemed until the
// worker credits it.
func TestPendingCashbackIsShownButNotRedeemable(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	e.launchGated(t, 1_000_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	e.pay(t, u, 3_500_000)

	pendingOf := func() int64 {
		rows, err := db.New(e.pool).ListAccounts(ctx, u.id)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.Kind == "cashback" {
				return r.PendingMinor
			}
		}
		t.Fatal("no cash back account listed")
		return 0
	}
	if p := pendingOf(); p != 175_000 {
		t.Errorf("listed pending = %d, want 175000", p)
	}
	_, err := Redeem(ctx, e.pool, RedeemParams{UserID: u.id, Currency: "IDR", AmountMinor: 175_000, IdempotencyKey: "redeem-pending-1"})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("redeeming pending cash back = %v, want ErrInsufficientFunds", err)
	}

	creditAll(t, e.pool)
	if p := pendingOf(); p != 0 {
		t.Errorf("listed pending = %d after crediting, want 0", p)
	}
	r, err := Redeem(ctx, e.pool, RedeemParams{UserID: u.id, Currency: "IDR", AmountMinor: 175_000, IdempotencyKey: "redeem-pending-2"})
	if err != nil {
		t.Fatal(err)
	}
	if r.CashbackBalanceMinor != 0 || r.CashbackPendingMinor != 0 {
		t.Errorf("after redeeming = balance %d, pending %d; want 0 and 0", r.CashbackBalanceMinor, r.CashbackPendingMinor)
	}
	assertLedger(t, e.pool)
}

// A batch is credited in one statement, with one credit per cash back
// account even when a user has several awards in it.
func TestWorkerCreditsBatchInOneStatement(t *testing.T) {
	e := newGateEnv(t)
	_, account := e.launchGated(t, 1_000_000_000, GateRedis)
	a, b := payingUser(t, e.pool, 1_000_000_000), payingUser(t, e.pool, 1_000_000_000)
	for range 3 {
		e.pay(t, a, 3_500_000)
		e.pay(t, b, 2_000_000)
	}

	ctx, stats := dbtrace.WithStats(context.Background())
	got, err := CreditPendingAwards(ctx, e.pool, AwardBatch)
	if err != nil {
		t.Fatal(err)
	}
	if got.Credited != 6 || got.Failed != 0 {
		t.Fatalf("worker = %+v, want all six credited", got)
	}
	// BEGIN, claim, lock the campaign, the batch, COMMIT.
	if n := stats.Queries(); n != 5 {
		t.Errorf("crediting six awards ran %d queries, want 5", n)
	}
	if bal := balanceOf(t, e.pool, a.cashback); bal != 3*175_000 {
		t.Errorf("a's cash back = %d, want 525000", bal)
	}
	if bal := balanceOf(t, e.pool, b.cashback); bal != 3*100_000 {
		t.Errorf("b's cash back = %d, want 300000", bal)
	}
	if bal := balanceOf(t, e.pool, account); bal != 1_000_000_000-825_000 {
		t.Errorf("campaign account = %d", bal)
	}
	assertLedger(t, e.pool)
}

// When the account can't cover a batch's total, the worker credits the
// awards it can cover and marks only the rest failed.
func TestWorkerFallsBackWhenBatchCantBeCovered(t *testing.T) {
	e := newGateEnv(t)
	ctx := context.Background()
	id, account := e.launchGated(t, 1_000_000, GateRedis)
	u := payingUser(t, e.pool, 1_000_000_000)
	var last PayResult
	for range 3 {
		last = e.pay(t, u, 3_500_000) // 175,000 each
	}
	// The last reservation claims more than the whole budget, as if Redis had
	// been seeded wrong. (More than what's left after the other two would
	// make the outcome depend on the order they're tried in.)
	if _, err := e.pool.Exec(ctx, "UPDATE cashback_pending SET amount_minor = 2000000 WHERE payment_transaction_id = $1",
		last.Transaction.ID); err != nil {
		t.Fatal(err)
	}

	got := creditAll(t, e.pool)
	if got.Credited != 2 || got.Failed != 1 {
		t.Fatalf("worker = %+v, want two credited and one failed", got)
	}
	if bal := balanceOf(t, e.pool, u.cashback); bal != 350_000 {
		t.Errorf("cash back = %d, want the two coverable awards (350000)", bal)
	}
	if bal := balanceOf(t, e.pool, account); bal != 650_000 {
		t.Errorf("campaign account = %d, want 650000", bal)
	}
	if p := pendingFor(t, e.pool, id); p != 2_000_000 {
		t.Errorf("pending = %d, want only the failed award", p)
	}
	assertLedger(t, e.pool)
}
