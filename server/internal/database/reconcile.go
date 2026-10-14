package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/budgetgate"
	"cashback/server/internal/db"
	"cashback/server/internal/logx"
)

const (
	// ReconcileInterval is how often the reconciler checks each gated campaign.
	ReconcileInterval = 5 * time.Second
	// reconcileLockTTL outlives the longest locked operation (a drain).
	reconcileLockTTL = GateSettleTime + 30*time.Second
	// driftAlertIncrements: drift of this many award increments or more is
	// logged at error rather than warn.
	driftAlertIncrements = 100
)

var (
	ErrGateNotReady = errors.New("budget gate can't recover yet")
	ErrCampaignOver = errors.New("campaign has ended")
	ErrBadGate      = errors.New("budget gate must be db, shadow or redis")
	// ErrGateNeeded: the operation needs Redis (a campaign using the gate).
	ErrGateNeeded = errors.New("campaign uses the Redis budget gate; Redis is needed")
)

// Reconciler keeps a campaign's Redis counter equal to what the database
// says is left, and switches campaigns between gates safely.
//
// The database is the truth: what's left is the campaign account balance
// minus pending awards (CampaignBudgetLeft). The counter should equal that
// minus reservations in flight (reserved in Redis, not yet committed).
type Reconciler struct {
	Pool *pgxpool.Pool
	Gate *budgetgate.Redis
}

// PassResult is what one reconcile pass found and did.
type PassResult struct {
	Open      bool  // the counter exists
	Drift     int64 // what the counter should gain (negative: it held too much)
	Corrected bool
	Pruned    int // committed reservations forgotten
	Released  int // leaked reservations returned
	InFlight  int // reservations still neither committed nor released
}

// lock takes the campaign's reconcile lock: one reconciler pass, switch or
// recovery at a time, across servers. Seeding the counter in the middle of
// a pass would make its arithmetic wrong.
func (r *Reconciler) lock(ctx context.Context, campaignID string) (func(), error) {
	b := make([]byte, 16)
	rand.Read(b)
	unlock, err := r.Gate.Lock(ctx, campaignID, hex.EncodeToString(b), reconcileLockTTL)
	if err != nil {
		return nil, err
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backgroundTimeout)
		defer cancel()
		unlock(ctx)
	}, nil
}

// Reconcile runs one pass for a campaign, correcting the counter if correct
// is set. Payments reserve and commit throughout; the pass reads Redis,
// then the database, then Redis again, and works out exactly what the
// counter held at the first read:
//
//	expected = left in the database
//	         - reservations listed at the first read and not committed
//	         + reservations made after it that had already committed
//
// The database is read in one REPEATABLE READ snapshot, so "left" and
// "committed" agree. Only reconcile passes remove committed reservations
// from Redis's in-flight list, so every reservation made since the last
// pass shows up in one of the two reads. Drift is then added with INCRBY,
// which keeps whatever reservations happened meanwhile.
//
// It also forgets committed reservations, and releases ones older than
// GateSettleTime that never reached the database (their payment crashed,
// rolled back without releasing, or timed out after Redis took them).
func (r *Reconciler) Reconcile(ctx context.Context, campaignID string, correct bool) (PassResult, error) {
	unlock, err := r.lock(ctx, campaignID)
	if err != nil {
		return PassResult{}, err
	}
	defer unlock()
	return r.pass(ctx, campaignID, correct)
}

func (r *Reconciler) pass(ctx context.Context, campaignID string, correct bool) (PassResult, error) {
	var out PassResult
	s1, err := r.Gate.Snapshot(ctx, campaignID)
	if err != nil {
		return out, err
	}
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	left, err := q.CampaignBudgetLeft(ctx, campaignID) // fixes the snapshot
	if err != nil {
		return out, fmt.Errorf("budget left: %w", err)
	}
	s2, err := r.Gate.Snapshot(ctx, campaignID)
	if err != nil {
		return out, err
	}
	ids := make([]string, 0, len(s1.Inflight)+len(s2.Inflight))
	for id := range s1.Inflight {
		ids = append(ids, id)
	}
	for id := range s2.Inflight {
		if _, ok := s1.Inflight[id]; !ok {
			ids = append(ids, id)
		}
	}
	committedIDs, err := q.CommittedReservations(ctx, ids)
	if err != nil {
		return out, fmt.Errorf("committed reservations: %w", err)
	}
	tx.Rollback(ctx)
	committed := make(map[string]bool, len(committedIDs))
	for _, id := range committedIDs {
		committed[id] = true
	}

	out.Open = s1.Open && s2.Open
	if out.Open {
		expected := left
		for id, in := range s1.Inflight {
			if !committed[id] {
				expected -= in.Amount
			}
		}
		for id, in := range s2.Inflight {
			if _, ok := s1.Inflight[id]; !ok && committed[id] {
				expected += in.Amount
			}
		}
		out.Drift = expected - s1.Remaining
		if correct && out.Drift != 0 {
			if out.Corrected, err = r.Gate.Adjust(ctx, campaignID, out.Drift); err != nil {
				return out, err
			}
		}
	}

	if err := r.Gate.Prune(ctx, campaignID, committedIDs); err != nil {
		return out, err
	}
	out.Pruned = len(committedIDs)
	for id, in := range s2.Inflight {
		if committed[id] {
			continue
		}
		if s2.Now.Sub(in.ReservedAt) <= GateSettleTime {
			out.InFlight++
			continue
		}
		if err := r.Gate.Release(ctx, campaignID, id); err != nil {
			return out, err
		}
		out.Released++
	}
	return out, nil
}

// Drain waits until no reservation is in flight: each has committed or
// been released. Call it with the counter closed, so none are added.
func (r *Reconciler) drain(ctx context.Context, campaignID string) error {
	for {
		res, err := r.pass(ctx, campaignID, false)
		if err != nil {
			return err
		}
		if res.InFlight == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %d reservations: %w", res.InFlight, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Recover reopens a degraded campaign's gate: with the campaign locked, so
// no payment awards meanwhile, it seeds the counter from the database and
// clears the degraded flag. Payments waiting on the lock see the flag
// cleared and reserve instead. It refuses until the gate has been degraded
// for GateSettleTime: before that, a payment that reserved earlier may
// still be committing, and seeding would forget its reservation.
func (r *Reconciler) Recover(ctx context.Context, campaignID string) error {
	unlock, err := r.lock(ctx, campaignID)
	if err != nil {
		return err
	}
	defer unlock()

	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	c, err := q.LockCampaign(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoCampaign
	}
	if err != nil {
		return err
	}
	switch {
	case c.BudgetGate != GateRedis || c.GateDegradedAt == nil:
		return nil // not degraded (any more)
	case c.EndsAt != nil && !c.EndsAt.After(c.DbNow):
		return ErrCampaignOver
	case c.DbNow.Sub(*c.GateDegradedAt) < GateSettleTime:
		return ErrGateNotReady
	}
	pending, err := q.PendingForCampaign(ctx, campaignID)
	if err != nil {
		return err
	}
	if err := r.Gate.Seed(ctx, campaignID, c.BalanceMinor-pending); err != nil {
		return err
	}
	if err := q.ClearGateDegraded(ctx, campaignID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		// Redis is open but payments still see the gate degraded and award
		// under the lock; the next recovery reseeds.
		return fmt.Errorf("commit recovery: %w", err)
	}
	slog.Info("budget gate recovered", "campaign_id", campaignID)
	return nil
}

// SetGate switches how a campaign's budget is enforced, without ever
// letting two paths spend the same budget:
//
//   - to shadow or redis: with the campaign locked (no award can happen),
//     seed the counter with what the database says is left, then switch.
//     Payments waiting on the lock see the new gate.
//   - from redis to db: close the counter (new reservations get nothing),
//     wait for every reservation in flight to commit or be released, then
//     switch with the campaign locked. Pending awards keep draining, and
//     the lock path subtracts them. Payments earn nothing meanwhile, for
//     at most GateSettleTime and usually milliseconds.
//   - from shadow to db: switch; shadow reservations never spent anything.
//
// r.Gate may be nil only to switch a degraded campaign (whose payments
// stopped reserving at least GateSettleTime ago) from redis to db.
func (r *Reconciler) SetGate(ctx context.Context, campaignID, gate string) error {
	if gate != GateDB && gate != GateShadow && gate != GateRedis {
		return ErrBadGate
	}
	current, err := r.currentGate(ctx, campaignID)
	if err != nil {
		return err
	}
	if current.gate == gate && !current.degraded {
		return nil
	}

	if r.Gate == nil {
		if current.gate == GateRedis && gate == GateDB && current.degraded && current.settled {
			return r.switchLocked(ctx, campaignID, current.gate, gate, false)
		}
		return ErrGateNeeded
	}
	unlock, err := r.lock(ctx, campaignID)
	if err != nil {
		return err
	}
	defer unlock()

	if current.gate == GateRedis && !(current.degraded && current.settled) {
		// Close and drain before anything else spends the budget.
		if err := r.Gate.Close(ctx, campaignID); err != nil {
			return err
		}
		dctx, cancel := context.WithTimeout(ctx, GateSettleTime+5*time.Second)
		err := r.drain(dctx, campaignID)
		cancel()
		if err != nil {
			return fmt.Errorf("drain reservations: %w", err)
		}
	}
	return r.switchLocked(ctx, campaignID, current.gate, gate, true)
}

type gateState struct {
	gate              string
	degraded, settled bool
}

func (r *Reconciler) currentGate(ctx context.Context, campaignID string) (gateState, error) {
	// Outside a transaction, the lock lasts only for the statement.
	c, err := db.New(r.Pool).LockCampaign(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return gateState{}, ErrNoCampaign
	}
	if err != nil {
		return gateState{}, err
	}
	if c.EndsAt != nil && !c.EndsAt.After(c.DbNow) {
		return gateState{}, ErrCampaignOver
	}
	s := gateState{gate: c.BudgetGate, degraded: c.GateDegradedAt != nil}
	s.settled = s.degraded && c.DbNow.Sub(*c.GateDegradedAt) >= GateSettleTime
	return s, nil
}

// switchLocked makes the switch with the campaign locked, seeding the
// counter when the new gate uses it.
func (r *Reconciler) switchLocked(ctx context.Context, campaignID, from, to string, withRedis bool) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	c, err := q.LockCampaign(ctx, campaignID)
	if err != nil {
		return err
	}
	if c.BudgetGate != from {
		return fmt.Errorf("budget gate changed to %s meanwhile; try again", c.BudgetGate)
	}
	if to != GateDB {
		pending, err := q.PendingForCampaign(ctx, campaignID)
		if err != nil {
			return err
		}
		if err := r.Gate.Seed(ctx, campaignID, c.BalanceMinor-pending); err != nil {
			return err
		}
	}
	if err := q.SetBudgetGate(ctx, db.SetBudgetGateParams{ID: campaignID, BudgetGate: to}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit gate switch: %w", err)
	}
	if to == GateDB && withRedis {
		// Shadow reservations still arriving get Closed and are ignored.
		if err := r.Gate.Close(ctx, campaignID); err != nil {
			slog.Warn("budget gate close after switching to db failed", "campaign_id", campaignID, logx.Err(err))
		}
	}
	slog.Info("budget gate switched", "campaign_id", campaignID, "from", from, "to", to)
	return nil
}

// RunReconciler reconciles every live gated campaign each interval, and
// recovers degraded ones once Redis answers, until ctx is done.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	log := slog.Default().With("component", "budget_reconciler")
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		campaigns, err := db.New(r.Pool).ListGatedCampaigns(ctx)
		if err != nil {
			log.Warn("list gated campaigns failed", logx.Err(err))
			continue
		}
		for _, c := range campaigns {
			r.reconcileOne(ctx, log.With("campaign_id", c.ID), c)
		}
	}
}

func (r *Reconciler) reconcileOne(ctx context.Context, log *slog.Logger, c db.ListGatedCampaignsRow) {
	if c.GateDegraded {
		err := r.Recover(ctx, c.ID)
		if err != nil && !errors.Is(err, ErrGateNotReady) && !errors.Is(err, budgetgate.ErrLocked) {
			log.Warn("budget gate still degraded", logx.Err(err))
		}
		return
	}
	res, err := r.Reconcile(ctx, c.ID, true)
	switch {
	case errors.Is(err, budgetgate.ErrLocked):
		return
	case err != nil:
		log.Warn("budget reconcile failed", logx.Err(err))
		return
	}
	if res.Drift != 0 {
		level := slog.LevelWarn
		if abs(res.Drift) >= driftAlertIncrements*max(c.AwardIncrementMinor, 1) {
			level = slog.LevelError
		}
		log.Log(ctx, level, "budget gate drift", "budget_gate", c.BudgetGate, "drift_minor", res.Drift, "corrected", res.Corrected)
	}
	if res.Released > 0 {
		log.Warn("budget gate released leaked reservations", "released", res.Released)
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
