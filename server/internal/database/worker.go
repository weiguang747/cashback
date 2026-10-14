package database

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/db"
	"cashback/server/internal/logx"
)

const (
	// AwardBatch is how many pending awards one worker transaction credits.
	// Each batch locks the campaign once, however many awards it holds.
	AwardBatch = 100
	// workerPoll is how often an idle worker looks for new awards: the
	// usual delay before a reserved award shows in the cash back balance.
	workerPoll = 50 * time.Millisecond
	// workerMaxBackoff caps the wait after a failed batch.
	workerMaxBackoff = 5 * time.Second
	// BacklogAlert: the worker logs at error while an award has waited longer.
	BacklogAlert = 30 * time.Second
	backlogCheck = 10 * time.Second
)

// CreditResult counts what one batch did.
type CreditResult struct {
	Credited int
	Failed   int // can never be credited; left for someone to resolve
	Ended    []string
}

// CreditPendingAwards credits up to limit pending awards in one database
// transaction: each moves from cashback_pending to cashback_awards, with
// its two ledger rows and balance changes (RecordAward), or is marked
// failed if the campaign account can't cover it.
//
// Locks follow Pay's order: the claimed pending rows, then each campaign
// and its account (in id order), then the users' cash back accounts. If the
// worker dies mid-batch the transaction rolls back and the awards are
// claimed again; cashback_awards' unique payment id means an award can't be
// credited twice.
func CreditPendingAwards(ctx context.Context, pool *pgxpool.Pool, limit int) (CreditResult, error) {
	var out CreditResult
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	rows, err := q.ClaimPendingAwards(ctx, int32(limit))
	if err != nil {
		return out, fmt.Errorf("claim pending awards: %w", err)
	}
	if len(rows) == 0 {
		return out, nil
	}

	campaigns := map[string]bool{}
	for _, r := range rows {
		campaigns[r.CampaignID] = true
	}
	ids := make([]string, 0, len(campaigns))
	for id := range campaigns {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if _, err := q.LockCampaign(ctx, id); err != nil {
			return out, fmt.Errorf("lock campaign %s: %w", id, err)
		}
	}

	slices.SortStableFunc(rows, func(a, b db.ClaimPendingAwardsRow) int { return cmp.Compare(a.CampaignID, b.CampaignID) })
	now := time.Now()
	var failed []string
	for _, r := range rows {
		award, err := q.RecordAward(ctx, db.RecordAwardParams{
			AmountMinor:          r.AmountMinor,
			CampaignAccountID:    r.CampaignAccountID,
			DebitDescription:     "Award for payment " + r.PaymentTransactionID,
			Currency:             r.Currency,
			At:                   now,
			CashbackAccountID:    r.CashbackAccountID,
			CreditDescription:    r.CreditDescription,
			CampaignID:           r.CampaignID,
			UserID:               r.UserID,
			PaymentTransactionID: r.PaymentTransactionID,
			CampaignDay:          r.CampaignDay,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// The campaign account can't cover it: the gate granted more than
			// the database holds. Never retried; the budget stays reserved.
			if err := q.MarkPendingFailed(ctx, r.PaymentTransactionID); err != nil {
				return out, fmt.Errorf("mark pending award failed: %w", err)
			}
			failed = append(failed, r.PaymentTransactionID)
			continue
		}
		if err != nil {
			return out, &awardError{paymentID: r.PaymentTransactionID, err: err}
		}
		if err := q.DeletePendingAward(ctx, r.PaymentTransactionID); err != nil {
			return out, fmt.Errorf("delete pending award: %w", err)
		}
		out.Credited++
		if award.CampaignEnded {
			out.Ended = append(out.Ended, r.CampaignID)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CreditResult{}, fmt.Errorf("commit awards: %w", err)
	}
	out.Failed = len(failed)
	for _, id := range failed {
		slog.Error("pending award failed: campaign account can't cover it", "payment_transaction_id", id)
	}
	return out, nil
}

// awardError is a database error writing one award.
type awardError struct {
	paymentID string
	err       error
}

func (e *awardError) Error() string {
	return fmt.Sprintf("record award for payment %s: %v", e.paymentID, e.err)
}
func (e *awardError) Unwrap() error { return e.err }

// creditOneByOne retries a batch that failed with a database error one
// award per transaction, so one bad award can't block the queue: an award
// that fails on its own with a database error is marked failed. Other
// errors (a lost connection) stop it, to be retried later.
func creditOneByOne(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	credited := 0
	for range AwardBatch {
		r, err := CreditPendingAwards(ctx, pool, 1)
		credited += r.Credited
		logEnded(r)
		var ae *awardError
		var pgErr *pgconn.PgError
		switch {
		case errors.As(err, &ae) && errors.As(err, &pgErr):
			if err := db.New(pool).MarkPendingFailed(ctx, ae.paymentID); err != nil {
				return credited, fmt.Errorf("mark pending award failed: %w", err)
			}
			slog.Error("pending award failed", "payment_transaction_id", ae.paymentID, logx.Err(err))
		case err != nil:
			return credited, err
		case r.Credited == 0 && r.Failed == 0:
			return credited, nil
		}
	}
	return credited, nil
}

func logEnded(r CreditResult) {
	for _, id := range r.Ended {
		slog.Info("campaign ended: budget used up", "campaign_id", id)
	}
}

// RunAwardWorker credits pending awards until ctx is done. Several servers
// can each run one; they share the queue (SKIP LOCKED).
func RunAwardWorker(ctx context.Context, pool *pgxpool.Pool) {
	log := slog.Default().With("component", "award_worker")
	backoff := workerPoll
	nextCheck := time.Now()
	for {
		r, err := CreditPendingAwards(ctx, pool, AwardBatch)
		logEnded(r)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			log.Warn("award batch failed; crediting one at a time", logx.Err(err))
			_, err = creditOneByOne(ctx, pool)
		}
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			log.Error("award batch failed", logx.Err(err), "retry_in_ms", backoff.Milliseconds())
			backoff = min(backoff*2, workerMaxBackoff)
		default:
			backoff = workerPoll
		}

		if time.Now().After(nextCheck) {
			checkBacklog(ctx, pool, log)
			nextCheck = time.Now().Add(backlogCheck)
		}
		if err == nil && r.Credited+r.Failed == AwardBatch {
			continue // more waiting
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// checkBacklog logs at error while awards wait too long or have failed.
func checkBacklog(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) {
	q := db.New(pool)
	age, err := q.OldestPendingAgeSecs(ctx)
	if err == nil && time.Duration(age)*time.Second > BacklogAlert {
		log.Error("cash back backlog: awards waiting to be credited", "oldest_age_s", age)
	}
	failed, err := q.CountFailedPendingAwards(ctx)
	if err == nil && failed > 0 {
		log.Error("pending awards failed and need resolving", "failed", failed)
	}
}
