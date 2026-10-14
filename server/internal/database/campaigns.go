package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/db"
)

var (
	// ErrCampaignOverlap: another campaign in the currency covers that time.
	ErrCampaignOverlap = errors.New("another campaign in this currency is scheduled for that time")
	ErrNoCampaign      = errors.New("no such campaign")
	ErrBadTimezone     = errors.New("unknown time zone")
)

type LaunchParams struct {
	Name                string
	Currency            string
	RateBps             int32
	MinSpendMinor       int64
	DailyCapMinor       int64
	AwardIncrementMinor int64
	BudgetMinor         int64
	Timezone            string
	StartsAt            time.Time
	EndsAt              *time.Time
}

// LaunchCampaign creates a campaign and funds it: the budget moves from the
// currency's funding account into a new campaign account, as a pair of
// transactions, in one database transaction.
func LaunchCampaign(ctx context.Context, pool *pgxpool.Pool, p LaunchParams) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	if err := q.ValidateTimezone(ctx, p.Timezone); err != nil {
		return "", fmt.Errorf("%w: %q", ErrBadTimezone, p.Timezone)
	}
	fundingID, err := q.UpsertFundingAccount(ctx, p.Currency)
	if err != nil {
		return "", fmt.Errorf("funding account: %w", err)
	}
	accountID, err := q.CreateCampaignAccount(ctx, p.Currency)
	if err != nil {
		return "", fmt.Errorf("campaign account: %w", err)
	}
	if err := transfer(ctx, q, fundingID, accountID, p.Currency, p.BudgetMinor, "Campaign funding", p.Name); err != nil {
		return "", err
	}
	id, err := q.CreateCampaign(ctx, db.CreateCampaignParams{
		Name: p.Name, AccountID: accountID, Currency: p.Currency, RateBps: p.RateBps,
		MinSpendMinor: p.MinSpendMinor, DailyCapMinor: p.DailyCapMinor, AwardIncrementMinor: p.AwardIncrementMinor,
		BudgetMinor: p.BudgetMinor, Timezone: p.Timezone, StartsAt: p.StartsAt, EndsAt: p.EndsAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "campaigns_no_overlap" {
		return "", ErrCampaignOverlap
	}
	if err != nil {
		return "", fmt.Errorf("create campaign: %w", err)
	}
	return id, tx.Commit(ctx)
}

// EndCampaign ends a campaign now and returns any unspent budget to the
// funding account. It returns the amount returned, in minor units.
//
// A campaign using the Redis gate is switched back to db first (r does
// that; see Reconciler.SetGate), so no reservation is in flight when the
// leftover is worked out. Awards reserved but not yet credited stay in the
// campaign account for the worker; only the rest goes back.
func EndCampaign(ctx context.Context, pool *pgxpool.Pool, r *Reconciler, id string) (int64, error) {
	if err := endGate(ctx, pool, r, id); err != nil {
		return 0, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	// Lock the campaign first, as payments do, so no award races the sweep.
	c, err := q.LockCampaign(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoCampaign
	}
	if err != nil {
		return 0, err
	}
	if err := q.CloseCampaign(ctx, db.CloseCampaignParams{ID: id, At: time.Now()}); err != nil {
		return 0, fmt.Errorf("end campaign: %w", err)
	}
	if c.BudgetGate != GateDB && (c.EndsAt == nil || c.EndsAt.After(c.DbNow)) {
		return 0, fmt.Errorf("budget gate switched to %s meanwhile; try again", c.BudgetGate)
	}
	// The worker can't credit these while the campaign is locked, so the
	// sum is exact.
	pending, err := q.PendingForCampaign(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("pending awards: %w", err)
	}
	leftover := c.BalanceMinor - pending
	if leftover > 0 {
		fundingID, err := q.GetFundingAccount(ctx, c.Currency)
		if err != nil {
			return 0, fmt.Errorf("funding account: %w", err)
		}
		if err := transfer(ctx, q, c.AccountID, fundingID, c.Currency, leftover, "Campaign ended", c.Name); err != nil {
			return 0, err
		}
	}
	return max(leftover, 0), tx.Commit(ctx)
}

// endGate switches a live campaign that uses the Redis gate back to db.
func endGate(ctx context.Context, pool *pgxpool.Pool, r *Reconciler, id string) error {
	// Outside a transaction, the lock lasts only for the statement.
	c, err := db.New(pool).LockCampaign(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoCampaign
	}
	if err != nil {
		return err
	}
	if c.BudgetGate == GateDB || (c.EndsAt != nil && !c.EndsAt.After(c.DbNow)) {
		return nil
	}
	if r == nil {
		return ErrGateNeeded
	}
	if err := r.SetGate(ctx, id, GateDB); err != nil && !errors.Is(err, ErrCampaignOver) {
		return fmt.Errorf("switch budget gate to db: %w", err)
	}
	return nil
}

// transfer moves amount between two accounts as a pair of transactions.
func transfer(ctx context.Context, q *db.Queries, from, to, currency string, amount int64, name, description string) error {
	now := time.Now()
	for _, leg := range []struct {
		account, kind string
		delta         int64
	}{{from, "out", -amount}, {to, "in", amount}} {
		if _, err := q.AddToBalance(ctx, db.AddToBalanceParams{ID: leg.account, DeltaMinor: leg.delta}); err != nil {
			return fmt.Errorf("move budget: %w", err)
		}
		if _, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
			AccountID: leg.account, Name: name, Description: description, Kind: leg.kind,
			AmountMinor: leg.delta, Currency: currency, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("record budget move: %w", err)
		}
	}
	return nil
}
