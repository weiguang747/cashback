package budgetgate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Runs against a real Redis only when TEST_REDIS_URL is set, e.g.
//
//	TEST_REDIS_URL=redis://localhost:6379/15 go test ./...
//
// Each test gets its own campaign id and deletes its keys afterwards.
func newTestGate(t *testing.T) (*Redis, *redis.Client, string) {
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
	campaign := fmt.Sprintf("test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := client.Keys(ctx, "campaign:{"+campaign+"}:*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		client.Close()
	})
	return NewRedis(client), client, campaign
}

func reserve(t *testing.T, g *Redis, campaign, payment string, want, increment int64) Grant {
	t.Helper()
	grant, err := g.Reserve(context.Background(), campaign, payment, want, increment)
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func remaining(t *testing.T, g *Redis, campaign string) int64 {
	t.Helper()
	s, err := g.Snapshot(context.Background(), campaign)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Open {
		t.Fatal("counter is closed")
	}
	return s.Remaining
}

func seed(t *testing.T, g *Redis, campaign string, budget int64) {
	t.Helper()
	if err := g.Seed(context.Background(), campaign, budget); err != nil {
		t.Fatal(err)
	}
}

func TestReserveGrants(t *testing.T) {
	g, _, c := newTestGate(t)
	seed(t, g, c, 10_000)

	got := reserve(t, g, c, "p1", 2_500, 1)
	if got != (Grant{Outcome: Granted, Amount: 2_500, Left: 7_500}) {
		t.Fatalf("Reserve = %+v, want 2500 granted, 7500 left", got)
	}
	if r := remaining(t, g, c); r != 7_500 {
		t.Errorf("remaining = %d, want 7500", r)
	}
	s, _ := g.Snapshot(context.Background(), c)
	if in := s.Inflight["p1"]; in.Amount != 2_500 || time.Since(in.ReservedAt) > time.Minute {
		t.Errorf("inflight[p1] = %+v, want 2500 reserved just now", in)
	}
}

func TestReserveGrantsWhatsLeftAtTheEdge(t *testing.T) {
	g, _, c := newTestGate(t)
	seed(t, g, c, 1_000)

	if got := reserve(t, g, c, "p1", 700, 1); got.Outcome != Granted || got.Left != 300 {
		t.Fatalf("first = %+v", got)
	}
	got := reserve(t, g, c, "p2", 700, 1)
	if got != (Grant{Outcome: Granted, Amount: 300, Left: 0}) {
		t.Fatalf("second = %+v, want the 300 left and the budget at exactly 0", got)
	}
	if got := reserve(t, g, c, "p3", 700, 1); got.Outcome != Exhausted {
		t.Errorf("third = %+v, want Exhausted", got)
	}
}

func TestReserveExactBudget(t *testing.T) {
	g, _, c := newTestGate(t)
	seed(t, g, c, 500)

	if got := reserve(t, g, c, "p1", 500, 100); got != (Grant{Outcome: Granted, Amount: 500, Left: 0}) {
		t.Fatalf("Reserve = %+v, want all 500, nothing left", got)
	}
	if got := reserve(t, g, c, "p2", 100, 100); got.Outcome != Exhausted {
		t.Errorf("after exact spend = %+v, want Exhausted", got)
	}
}

func TestReserveZeroBudget(t *testing.T) {
	g, _, c := newTestGate(t)
	seed(t, g, c, 0)

	if got := reserve(t, g, c, "p1", 100, 1); got.Outcome != Exhausted {
		t.Errorf("Reserve = %+v, want Exhausted", got)
	}
	if r := remaining(t, g, c); r != 0 {
		t.Errorf("remaining = %d, want 0", r)
	}
}

func TestReserveRoundsDownToIncrement(t *testing.T) {
	g, _, c := newTestGate(t)
	seed(t, g, c, 1_250)

	// 1,250 left, whole-rupiah awards (100 sen): grant 1,200, keep 50.
	if got := reserve(t, g, c, "p1", 5_000, 100); got != (Grant{Outcome: Granted, Amount: 1_200, Left: 50}) {
		t.Fatalf("Reserve = %+v, want 1200 granted, 50 left", got)
	}
	if got := reserve(t, g, c, "p2", 100, 100); got.Outcome != Exhausted {
		t.Errorf("with less than one increment left = %+v, want Exhausted", got)
	}
}

func TestReserveDuplicatePayment(t *testing.T) {
	g, _, c := newTestGate(t)
	seed(t, g, c, 1_000)

	reserve(t, g, c, "p1", 400, 1)
	if got := reserve(t, g, c, "p1", 400, 1); got != (Grant{Outcome: Duplicate, Amount: 400}) {
		t.Fatalf("repeat = %+v, want Duplicate of 400", got)
	}
	if r := remaining(t, g, c); r != 600 {
		t.Errorf("remaining = %d, want 600 (reserved once)", r)
	}
}

func TestReserveClosed(t *testing.T) {
	g, _, c := newTestGate(t)

	if got := reserve(t, g, c, "p1", 100, 1); got.Outcome != Closed {
		t.Errorf("never seeded = %+v, want Closed", got)
	}
	seed(t, g, c, 1_000)
	if err := g.Close(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := reserve(t, g, c, "p2", 100, 1); got.Outcome != Closed {
		t.Errorf("after Close = %+v, want Closed", got)
	}
}

func TestReleaseOnce(t *testing.T) {
	g, _, c := newTestGate(t)
	ctx := context.Background()
	seed(t, g, c, 1_000)
	reserve(t, g, c, "p1", 400, 1)

	for range 3 {
		if err := g.Release(ctx, c, "p1"); err != nil {
			t.Fatal(err)
		}
	}
	if r := remaining(t, g, c); r != 1_000 {
		t.Errorf("remaining = %d, want 1000 (returned exactly once)", r)
	}
	if err := g.Release(ctx, c, "never-reserved"); err != nil {
		t.Fatal(err)
	}
	if r := remaining(t, g, c); r != 1_000 {
		t.Errorf("releasing an unknown payment changed the counter to %d", r)
	}
	if got := reserve(t, g, c, "p1", 400, 1); got != (Grant{Outcome: Duplicate}) {
		t.Errorf("reserving a released payment again = %+v, want Duplicate of 0", got)
	}
	if s, _ := g.Snapshot(ctx, c); len(s.Inflight) != 0 {
		t.Errorf("inflight after release = %v, want empty", s.Inflight)
	}
}

func TestReleaseAfterCloseKeepsCounterClosed(t *testing.T) {
	g, _, c := newTestGate(t)
	ctx := context.Background()
	seed(t, g, c, 1_000)
	reserve(t, g, c, "p1", 400, 1)

	if err := g.Close(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := g.Release(ctx, c, "p1"); err != nil {
		t.Fatal(err)
	}
	s, err := g.Snapshot(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Open {
		t.Errorf("Release reopened a closed counter at %d", s.Remaining)
	}
	if len(s.Inflight) != 0 {
		t.Errorf("inflight = %v, want the released payment gone", s.Inflight)
	}
}

func TestReserveConcurrentNeverOverspends(t *testing.T) {
	g, _, c := newTestGate(t)
	const budget, award, payments = 10_000, 300, 200
	seed(t, g, c, budget)

	var granted atomic.Int64
	var wg sync.WaitGroup
	for i := range payments {
		wg.Go(func() {
			got, err := g.Reserve(context.Background(), c, fmt.Sprintf("p%d", i), award, 1)
			if err != nil {
				t.Error(err)
				return
			}
			granted.Add(got.Amount)
		})
	}
	wg.Wait()

	if granted.Load() != budget {
		t.Errorf("granted %d in total, want exactly the budget %d", granted.Load(), budget)
	}
	if r := remaining(t, g, c); r != 0 {
		t.Errorf("remaining = %d, want 0", r)
	}
	s, _ := g.Snapshot(context.Background(), c)
	var inflight int64
	for _, in := range s.Inflight {
		inflight += in.Amount
	}
	if inflight != budget {
		t.Errorf("inflight sums to %d, want %d", inflight, budget)
	}
}

func TestSeedAdjustPrune(t *testing.T) {
	g, _, c := newTestGate(t)
	ctx := context.Background()

	if ok, err := g.Adjust(ctx, c, 100); err != nil || ok {
		t.Fatalf("Adjust on a closed counter = %v, %v; want false (nothing created)", ok, err)
	}
	seed(t, g, c, 1_000)
	reserve(t, g, c, "p1", 100, 1)
	reserve(t, g, c, "p2", 200, 1)

	if ok, err := g.Adjust(ctx, c, -50); err != nil || !ok {
		t.Fatalf("Adjust = %v, %v", ok, err)
	}
	if r := remaining(t, g, c); r != 650 {
		t.Errorf("remaining = %d, want 650", r)
	}
	if err := g.Prune(ctx, c, []string{"p1"}); err != nil {
		t.Fatal(err)
	}
	s, _ := g.Snapshot(ctx, c)
	if _, ok := s.Inflight["p1"]; ok || len(s.Inflight) != 1 {
		t.Errorf("inflight after prune = %v, want only p2", s.Inflight)
	}

	seed(t, g, c, 5_000)
	s, _ = g.Snapshot(ctx, c)
	if s.Remaining != 5_000 || len(s.Inflight) != 0 {
		t.Errorf("after Seed = %+v, want 5000 and nothing in flight", s)
	}
}

func TestLockIsExclusive(t *testing.T) {
	g, _, c := newTestGate(t)
	ctx := context.Background()

	unlock, err := g.Lock(ctx, c, "a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Lock(ctx, c, "b", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Lock = %v, want ErrLocked", err)
	}
	unlock(ctx)
	unlock2, err := g.Lock(ctx, c, "b", time.Minute)
	if err != nil {
		t.Fatalf("Lock after unlock = %v", err)
	}
	unlock2(ctx)
}

func TestLargeAmountsStayExact(t *testing.T) {
	g, _, c := newTestGate(t)
	// A trillion rupiah budget in sen, beyond what %.14g prints exactly.
	const budget = 100_000_000_000_000
	seed(t, g, c, budget)

	got := reserve(t, g, c, "p1", 123_456_789_012_345-budget/2, 1)
	if got.Outcome != Granted || got.Amount != 123_456_789_012_345-budget/2 || got.Left != budget-got.Amount {
		t.Fatalf("Reserve = %+v", got)
	}
	s, _ := g.Snapshot(context.Background(), c)
	if s.Inflight["p1"].Amount != got.Amount || s.Remaining != got.Left {
		t.Errorf("snapshot = %+v, want amounts kept exactly", s)
	}
}
