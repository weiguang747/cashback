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
	payment, err := q.InsertPayment(ctx, db.InsertPaymentParams{
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

	cb, err := awardCashback(ctx, q, p, payment.ID, now)
	if err != nil {
		return PayResult{}, fmt.Errorf("cash back: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PayResult{}, fmt.Errorf("commit payment: %w", err)
	}
	return PayResult{Transaction: db.GetTransactionByIdempotencyKeyRow(payment), BalanceMinor: balance, Cashback: cb}, nil
}

// awardCashback applies the campaign live at the payment's time, if any.
// An award moves money from the campaign account to the user's cash back
// account as a pair of transactions, linked to the payment by a
// cashback_awards row. When the budget reaches zero the campaign ends.
//
// Every awarding payment in a currency waits for the one campaign row, so
// the time between locking it and committing limits payment throughput.
// It is two statements: LockActiveCampaign reads everything the award
// needs, and RecordAward writes it all. Keep it that way.
func awardCashback(ctx context.Context, q *db.Queries, p PayParams, paymentID string, now time.Time) (*CashbackResult, error) {
	campaign, err := q.LockActiveCampaign(ctx, db.LockActiveCampaignParams{Currency: p.Currency, At: now, UserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find campaign: %w", err)
	}
	result := &CashbackResult{CampaignID: campaign.ID}

	amount, reason := cashback.Award(p.AmountMinor, cashback.Rules{
		RateBps:        int64(campaign.RateBps),
		MinSpend:       campaign.MinSpendMinor,
		DailyCap:       campaign.DailyCapMinor,
		AwardIncrement: campaign.AwardIncrementMinor,
	}, cashback.State{AwardedToday: campaign.AwardedTodayMinor, BudgetRemaining: campaign.BudgetMinor})
	result.Reason = reason
	if amount == 0 {
		return result, nil
	}
	result.AmountMinor = amount

	if campaign.CashbackAccountID == nil {
		return nil, errors.New("no cash back account")
	}
	award, err := q.RecordAward(ctx, db.RecordAwardParams{
		AmountMinor:          amount,
		CampaignAccountID:    campaign.AccountID,
		DebitDescription:     "Award for payment " + paymentID,
		Currency:             p.Currency,
		At:                   now,
		CashbackAccountID:    *campaign.CashbackAccountID,
		CreditDescription:    "From " + p.MerchantName,
		CampaignID:           campaign.ID,
		UserID:               p.UserID,
		PaymentTransactionID: paymentID,
		CampaignDay:          campaign.CampaignDay,
	})
	if err != nil {
		// No rows would mean the locked budget couldn't cover the award,
		// which Award rules out; treat it like any other failure.
		return nil, fmt.Errorf("record award: %w", err)
	}
	result.CampaignEnded = award.CampaignEnded
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
