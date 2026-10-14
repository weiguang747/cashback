package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/budgetgate"
	"cashback/server/internal/cashback"
	"cashback/server/internal/db"
	"cashback/server/internal/logx"
)

// Budget gate modes (campaigns.budget_gate).
const (
	GateDB     = "db"     // award under the campaign row lock, in the payment
	GateShadow = "shadow" // as db, and mirror each award into Redis to compare
	GateRedis  = "redis"  // reserve in Redis; the award worker credits it later
)

const (
	// MaxPayDuration bounds a Pay transaction, whatever its caller's
	// deadline. A reservation taken by a payment that hasn't committed by
	// then never will be, which is what the timings below rely on.
	MaxPayDuration = 10 * time.Second
	// GateSettleTime is how long after a reservation its payment is sure to
	// have committed or rolled back: MaxPayDuration plus margin for a
	// COMMIT sent just before the deadline. After a gate degrades, payments
	// wait this long before awarding under the lock, and the reconciler
	// releases reservations older than this that never reached the database.
	GateSettleTime = MaxPayDuration + 5*time.Second
	// DefaultReserveTimeout bounds each Reserve call. It runs inside the
	// payment's transaction, holding the user's main account, so it's short;
	// a slow Redis costs the payment its cash back, not its latency.
	DefaultReserveTimeout = 50 * time.Millisecond

	// A campaign degrades to the database fallback after degradeAfter gate
	// failures within degradeWindow, in one server.
	degradeAfter  = 5
	degradeWindow = 10 * time.Second
	// backgroundTimeout bounds the Redis and database calls a payment
	// leaves running after it returns (release, shadow, degrade).
	backgroundTimeout = 2 * time.Second
)

// CashbackGate is how Pay uses the Redis budget gate. One per server.
type CashbackGate struct {
	Gate budgetgate.Gate
	// ReserveTimeout bounds each Reserve; zero means DefaultReserveTimeout.
	ReserveTimeout time.Duration

	pool *pgxpool.Pool
	mu   sync.Mutex
	// failures holds recent gate failure times per campaign.
	failures map[string][]time.Time
	// background tracks work a payment leaves running after it returns.
	background sync.WaitGroup
}

func NewCashbackGate(pool *pgxpool.Pool, gate budgetgate.Gate) *CashbackGate {
	return &CashbackGate{Gate: gate, pool: pool, failures: map[string][]time.Time{}}
}

// Wait blocks until background work started by payments has finished: on
// shutdown, and in tests before checking Redis.
func (g *CashbackGate) Wait() { g.background.Wait() }

func (g *CashbackGate) reserveTimeout() time.Duration {
	if g.ReserveTimeout > 0 {
		return g.ReserveTimeout
	}
	return DefaultReserveTimeout
}

// goBackground runs f after the payment returns, detached from its
// request (which may be canceled) but keeping its logger.
func (g *CashbackGate) goBackground(ctx context.Context, f func(ctx context.Context)) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backgroundTimeout)
	g.background.Go(func() {
		defer cancel()
		f(ctx)
	})
}

// release returns a reservation whose payment won't commit.
func (g *CashbackGate) release(ctx context.Context, campaignID, paymentID string) {
	g.goBackground(ctx, func(ctx context.Context) {
		if err := g.Gate.Release(ctx, campaignID, paymentID); err != nil {
			// The reconciler releases it once it's older than GateSettleTime.
			logx.From(ctx).Warn("budget gate release failed", "campaign_id", campaignID,
				"payment_transaction_id", paymentID, logx.Err(err))
		}
	})
}

// failed records a gate failure, and degrades the campaign to the database
// fallback once failures pile up.
func (g *CashbackGate) failed(ctx context.Context, campaignID string, err error) {
	log := logx.From(ctx)
	log.Warn("budget gate failed", "campaign_id", campaignID, logx.Err(err))

	now := time.Now()
	g.mu.Lock()
	recent := g.failures[campaignID][:0]
	for _, t := range g.failures[campaignID] {
		if now.Sub(t) < degradeWindow {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	trip := len(recent) >= degradeAfter
	if trip {
		recent = nil
	}
	g.failures[campaignID] = recent
	g.mu.Unlock()
	if !trip || g.pool == nil {
		return
	}

	// In its own transaction, not the payment's: the flag must stick even if
	// this payment rolls back. In the background, so the payment doesn't
	// need a second pool connection.
	g.goBackground(ctx, func(ctx context.Context) {
		_, err := db.New(g.pool).DegradeGate(ctx, campaignID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Already degraded, or no longer using the Redis gate.
		case err != nil:
			logx.From(ctx).Error("budget gate degrade failed", "campaign_id", campaignID, logx.Err(err))
		default:
			logx.From(ctx).Error("budget gate degraded: awarding under the campaign lock until Redis recovers",
				"campaign_id", campaignID, "failures", degradeAfter, "window_s", degradeWindow.Seconds())
		}
	})
}

// awardInput is what deciding an award needs, from GetActiveCampaign or
// LockActiveCampaign.
type awardInput struct {
	campaignID        string
	rules             cashback.Rules
	awardedToday      int64
	campaignDay       time.Time
	cashbackAccountID *string
}

// reservation is a Redis reservation the payment holds until it commits.
type reservation struct{ campaignID, paymentID string }

// reserveCashback awards through the Redis gate: reserve the amount, then
// record it in cashback_pending in the payment's transaction. The worker
// credits it after the payment commits. If the payment doesn't commit, Pay
// releases the reservation.
func reserveCashback(ctx context.Context, q *db.Queries, g *CashbackGate, in awardInput, p PayParams,
	paymentID string, now time.Time) (*CashbackResult, *reservation, error) {
	result := &CashbackResult{CampaignID: in.campaignID}
	// The gate applies the budget; Award applies the rate, minimum and cap.
	want, reason := cashback.Award(p.AmountMinor, in.rules,
		cashback.State{AwardedToday: in.awardedToday, BudgetRemaining: math.MaxInt64})
	result.Reason = reason
	if want == 0 {
		return result, nil, nil
	}
	if in.cashbackAccountID == nil {
		return noCashbackAccount(ctx, in.campaignID, p.UserID), nil, nil
	}
	if g == nil || g.Gate == nil {
		result.Reason = cashback.Unavailable
		return result, nil, nil
	}

	rctx, cancel := context.WithTimeout(ctx, g.reserveTimeout())
	grant, err := g.Gate.Reserve(rctx, in.campaignID, paymentID, want, in.rules.AwardIncrement)
	cancel()
	if err != nil {
		// Fail closed: no cash back, but the payment goes ahead. The script
		// may still have run (a timeout), so give back whatever it took.
		g.failed(ctx, in.campaignID, err)
		g.release(ctx, in.campaignID, paymentID)
		result.Reason = cashback.Unavailable
		return result, nil, nil
	}
	amount := grant.Amount
	switch grant.Outcome {
	case budgetgate.Exhausted:
		result.Reason = cashback.BudgetExhausted
		return result, nil, nil
	case budgetgate.Closed:
		// Switching the gate or ending the campaign; brief.
		result.Reason = cashback.Unavailable
		return result, nil, nil
	case budgetgate.Duplicate:
		// Payment ids are new for every attempt, so this is our own
		// reservation; one already released can't be used again.
		if amount == 0 {
			result.Reason = cashback.Unavailable
			return result, nil, nil
		}
	}

	res := &reservation{campaignID: in.campaignID, paymentID: paymentID}
	err = q.InsertPendingAward(ctx, db.InsertPendingAwardParams{
		PaymentTransactionID: paymentID,
		CampaignID:           in.campaignID,
		UserID:               p.UserID,
		CashbackAccountID:    *in.cashbackAccountID,
		Currency:             p.Currency,
		AmountMinor:          amount,
		CampaignDay:          in.campaignDay,
		CreditDescription:    "From " + p.MerchantName,
		CreatedAt:            now,
	})
	if err != nil {
		return nil, res, fmt.Errorf("record pending award: %w", err)
	}
	// A partial award at the budget's edge still counts as awarded, as on
	// the database path.
	result.AmountMinor, result.Reason, result.Pending = amount, cashback.Awarded, true
	return result, res, nil
}

// shadow mirrors an award made under the lock into the Redis counter and
// logs when Redis would have decided differently. Its result is never used.
func (g *CashbackGate) shadow(ctx context.Context, campaignID, paymentID string, awarded, increment int64) {
	if g == nil || g.Gate == nil || awarded == 0 {
		return
	}
	g.goBackground(ctx, func(ctx context.Context) {
		log := logx.From(ctx).With("campaign_id", campaignID, "payment_transaction_id", paymentID)
		grant, err := g.Gate.Reserve(ctx, campaignID, paymentID, awarded, increment)
		switch {
		case err != nil:
			log.Warn("budget gate shadow failed", logx.Err(err))
		case grant.Outcome == budgetgate.Closed:
			// Being switched; nothing to compare.
		case grant.Amount != awarded:
			log.Warn("budget gate disagreement", "db_minor", awarded, "redis_minor", grant.Amount,
				"redis_outcome", grant.Outcome.String())
		default:
			log.Debug("budget gate agreed", "amount_minor", awarded)
		}
	})
}
