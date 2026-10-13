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
func EndCampaign(ctx context.Context, pool *pgxpool.Pool, id string) (int64, error) {
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
	if c.BalanceMinor > 0 {
		fundingID, err := q.GetFundingAccount(ctx, c.Currency)
		if err != nil {
			return 0, fmt.Errorf("funding account: %w", err)
		}
		if err := transfer(ctx, q, c.AccountID, fundingID, c.Currency, c.BalanceMinor, "Campaign ended", c.Name); err != nil {
			return 0, err
		}
	}
	return c.BalanceMinor, tx.Commit(ctx)
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
