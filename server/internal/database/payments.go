package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/cashback"
	"cashback/server/internal/db"
)

var (
	// ErrInsufficientFunds: the account balance is lower than the amount.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrKeyReused: the idempotency key already paid for a different request.
	ErrKeyReused = errors.New("idempotency key already used for a different payment")
)

type PayParams struct {
	UserID         string
	AccountID      string // the user's main account
	Currency       string
	AmountMinor    int64 // positive; debited from the account
	MerchantName   string
	IdempotencyKey string
	// Now is when the payment happens; zero means time.Now(). Tests set it
	// to land either side of a campaign's midnight.
	Now time.Time
}

type PayResult struct {
	Transaction  db.GetTransactionByIdempotencyKeyRow
	BalanceMinor int64 // the account's balance after the payment
	// Cashback is nil when no campaign was live for the payment.
	Cashback *CashbackResult
	// Replayed is true when the key had already paid and nothing new was
	// charged (Redis lost the key; the database caught the retry).
	Replayed bool
}

type CashbackResult struct {
	CampaignID  string
	AmountMinor int64 // 0 when Reason says why nothing was earned
	Reason      cashback.Reason
	// CampaignEnded is true when this award used up the campaign's budget.
	CampaignEnded bool
}

// Pay debits the account, records the payment and awards any campaign
// cash back, all in one database transaction, so every stored balance
// always equals the sum of its transactions.
//
// Rows are locked in the same order every time to avoid deadlocks: the
// user's main account, then the campaign, then the user's cash back account.
//
// The conditional debit refuses to overdraw, and its row lock makes
// concurrent payments from one account run in turn. The unique
// (account_id, idempotency_key) constraint means a key can only ever pay
// once, even if the Redis idempotency layer is bypassed.
func Pay(ctx context.Context, pool *pgxpool.Pool, p PayParams) (PayResult, error) {
	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PayResult{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	balance, err := q.DebitAccount(ctx, db.DebitAccountParams{ID: p.AccountID, AmountMinor: p.AmountMinor})
	if errors.Is(err, pgx.ErrNoRows) {
		return PayResult{}, ErrInsufficientFunds
	}
	if err != nil {
		return PayResult{}, fmt.Errorf("debit: %w", err)
	}

	key := p.IdempotencyKey
	paymentID, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
		AccountID:      p.AccountID,
		Name:           p.MerchantName,
		Description:    "Payment",
		Kind:           "out",
		AmountMinor:    -p.AmountMinor,
		Currency:       p.Currency,
		CreatedAt:      now,
		IdempotencyKey: &key,
	})
	if isIdempotencyConflict(err) {
		// Undo this attempt's debit and return what the key already paid.
		tx.Rollback(ctx)
		return replay(ctx, pool, p)
	}
	if err != nil {
		return PayResult{}, fmt.Errorf("insert payment: %w", err)
	}

	cb, err := awardCashback(ctx, q, p, paymentID, now)
	if err != nil {
		return PayResult{}, fmt.Errorf("cash back: %w", err)
	}

	row, err := q.GetTransactionByIdempotencyKey(ctx, db.GetTransactionByIdempotencyKeyParams{AccountID: p.AccountID, IdempotencyKey: &key})
	if err != nil {
		return PayResult{}, fmt.Errorf("read payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PayResult{}, fmt.Errorf("commit payment: %w", err)
	}
	return PayResult{Transaction: row, BalanceMinor: balance, Cashback: cb}, nil
}

// awardCashback applies the campaign live at the payment's time, if any.
// An award moves money from the campaign account to the user's cash back
// account as a pair of transactions, linked to the payment by a
// cashback_awards row. When the budget reaches zero the campaign ends.
func awardCashback(ctx context.Context, q *db.Queries, p PayParams, paymentID string, now time.Time) (*CashbackResult, error) {
	campaign, err := q.LockActiveCampaign(ctx, db.LockActiveCampaignParams{Currency: p.Currency, At: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find campaign: %w", err)
	}
	result := &CashbackResult{CampaignID: campaign.ID}

	awardedToday, err := q.UserAwardedOnDay(ctx, db.UserAwardedOnDayParams{
		CampaignID: campaign.ID, UserID: p.UserID, CampaignDay: campaign.CampaignDay,
	})
	if err != nil {
		return nil, fmt.Errorf("awarded today: %w", err)
	}
	budget, err := q.GetAccountBalance(ctx, campaign.AccountID)
	if err != nil {
		return nil, fmt.Errorf("campaign budget: %w", err)
	}

	amount, reason := cashback.Award(p.AmountMinor, cashback.Rules{
		RateBps:        int64(campaign.RateBps),
		MinSpend:       campaign.MinSpendMinor,
		DailyCap:       campaign.DailyCapMinor,
		AwardIncrement: campaign.AwardIncrementMinor,
	}, cashback.State{AwardedToday: awardedToday, BudgetRemaining: budget})
	result.Reason = reason
	if amount == 0 {
		return result, nil
	}
	result.AmountMinor = amount

	cashbackAccount, err := q.GetUserAccount(ctx, db.GetUserAccountParams{UserID: p.UserID, Kind: "cashback", Currency: p.Currency})
	if err != nil {
		return nil, fmt.Errorf("cash back account: %w", err)
	}

	// Out of the budget (conditional, so it can never overdraw) ...
	budgetLeft, err := q.DebitAccount(ctx, db.DebitAccountParams{ID: campaign.AccountID, AmountMinor: amount})
	if err != nil {
		return nil, fmt.Errorf("debit campaign: %w", err)
	}
	debitID, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
		AccountID: campaign.AccountID, Name: "Cash back", Description: "Award for payment " + paymentID,
		Kind: "out", AmountMinor: -amount, Currency: p.Currency, CreatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("record campaign debit: %w", err)
	}

	// ... into the user's cash back account.
	if _, err := q.AddToBalance(ctx, db.AddToBalanceParams{ID: cashbackAccount.ID, DeltaMinor: amount}); err != nil {
		return nil, fmt.Errorf("credit cash back: %w", err)
	}
	creditID, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
		AccountID: cashbackAccount.ID, Name: "Cash back", Description: "From " + p.MerchantName,
		Kind: "cashback", AmountMinor: amount, Currency: p.Currency, CreatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("record cash back credit: %w", err)
	}

	if err := q.InsertCashbackAward(ctx, db.InsertCashbackAwardParams{
		CampaignID: campaign.ID, UserID: p.UserID, PaymentTransactionID: paymentID,
		DebitTransactionID: debitID, CreditTransactionID: creditID,
		AmountMinor: amount, CampaignDay: campaign.CampaignDay,
	}); err != nil {
		return nil, fmt.Errorf("record award: %w", err)
	}

	if budgetLeft == 0 {
		if err := q.CloseCampaign(ctx, db.CloseCampaignParams{ID: campaign.ID, At: now}); err != nil {
			return nil, fmt.Errorf("end campaign: %w", err)
		}
		result.CampaignEnded = true
	}
	return result, nil
}

// replay returns the transaction an idempotency key already created, with
// the account's current balance.
func replay(ctx context.Context, pool *pgxpool.Pool, p PayParams) (PayResult, error) {
	q := db.New(pool)
	key := p.IdempotencyKey
	row, err := q.GetTransactionByIdempotencyKey(ctx, db.GetTransactionByIdempotencyKeyParams{AccountID: p.AccountID, IdempotencyKey: &key})
	if err != nil {
		return PayResult{}, fmt.Errorf("read earlier payment: %w", err)
	}
	if row.AmountMinor != -p.AmountMinor || row.Name != p.MerchantName {
		return PayResult{}, ErrKeyReused
	}
	balance, err := q.GetAccountBalance(ctx, p.AccountID)
	if err != nil {
		return PayResult{}, fmt.Errorf("read balance: %w", err)
	}
	// The original reason for no award isn't stored, so only an actual award
	// can be reported back.
	var cb *CashbackResult
	award, err := q.GetAwardByPayment(ctx, row.ID)
	switch {
	case err == nil:
		cb = &CashbackResult{CampaignID: award.CampaignID, AmountMinor: award.AmountMinor, Reason: cashback.Awarded}
	case !errors.Is(err, pgx.ErrNoRows):
		return PayResult{}, fmt.Errorf("read earlier cash back: %w", err)
	}
	return PayResult{Transaction: row, BalanceMinor: balance, Cashback: cb, Replayed: true}, nil
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "transactions_account_idempotency_key"
}
