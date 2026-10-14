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
	// Gate is the Redis budget gate, for campaigns in shadow or redis mode.
	// Without one, a redis-mode campaign awards nothing (unavailable).
	Gate *CashbackGate
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
	// Pending is true when the award is reserved but not yet credited to the
	// user's cash back account (the award worker does that shortly).
	Pending bool
	// CampaignEnded is true when this award used up the campaign's budget.
	CampaignEnded bool
}

// Pay debits the account, records the payment and awards any campaign
// cash back, all in one database transaction, so every stored balance
// always equals the sum of its transactions.
//
// How cash back is awarded depends on the campaign's budget gate:
//   - db (and shadow): under the campaign row lock, in this transaction.
//   - redis: reserved in Redis and recorded in cashback_pending, in this
//     transaction; the award worker credits it after the payment commits.
//     When the gate is degraded, under the lock again (see GateSettleTime).
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
	// A reservation must be committed or rolled back within MaxPayDuration;
	// the reconciler and the degraded fallback rely on it.
	ctx, cancel := context.WithTimeout(ctx, MaxPayDuration)
	defer cancel()

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

	cb, after, err := awardCashback(ctx, q, p, payment.ID, now)
	if err == nil {
		err = tx.Commit(ctx)
		if err != nil {
			// The commit may have happened anyway (the connection dropped
			// after sending it), so keep any reservation: the reconciler
			// releases it if the database has no record of it.
			return PayResult{}, fmt.Errorf("commit payment: %w", err)
		}
		after.committed(ctx, p.Gate)
		return PayResult{Transaction: db.GetTransactionByIdempotencyKeyRow(payment), BalanceMinor: balance, Cashback: cb}, nil
	}
	// Not committed and never will be: give back any reservation.
	after.rolledBack(ctx, p.Gate)
	return PayResult{}, fmt.Errorf("cash back: %w", err)
}

// afterPay is what a payment must do with the gate once its transaction
// has ended: return a reservation if it rolled back, or mirror a
// shadow-mode award if it committed.
type afterPay struct {
	reserved *reservation
	shadow   *shadowAward
}

type shadowAward struct {
	campaignID, paymentID string
	amount, increment     int64
}

func (a afterPay) committed(ctx context.Context, g *CashbackGate) {
	if s := a.shadow; s != nil {
		g.shadow(ctx, s.campaignID, s.paymentID, s.amount, s.increment)
	}
}

func (a afterPay) rolledBack(ctx context.Context, g *CashbackGate) {
	if r := a.reserved; r != nil && g != nil {
		g.release(ctx, r.campaignID, r.paymentID)
	}
}

// awardCashback applies the campaign live at the payment's time, if any,
// through its budget gate.
func awardCashback(ctx context.Context, q *db.Queries, p PayParams, paymentID string, now time.Time) (*CashbackResult, afterPay, error) {
	// Choosing the path needs no lock, so it happens before taking one.
	c, err := q.GetActiveCampaign(ctx, db.GetActiveCampaignParams{
		Currency: p.Currency, At: now, UserID: p.UserID, FallbackAfterSecs: int32(GateSettleTime / time.Second),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, afterPay{}, nil
	}
	if err != nil {
		return nil, afterPay{}, fmt.Errorf("find campaign: %w", err)
	}

	if c.BudgetGate == GateRedis && !c.GateDegraded {
		cb, res, err := reserveCashback(ctx, q, p.Gate, awardInput{
			campaignID:        c.ID,
			rules:             rules(c.RateBps, c.MinSpendMinor, c.DailyCapMinor, c.AwardIncrementMinor),
			awardedToday:      c.AwardedTodayMinor,
			campaignDay:       c.CampaignDay,
			cashbackAccountID: c.CashbackAccountID,
		}, p, paymentID, now)
		return cb, afterPay{reserved: res}, err
	}
	if c.BudgetGate == GateRedis && !c.FallbackReady {
		// Degraded moments ago: payments that reserved before that may still
		// be committing, and awarding under the lock now could spend budget
		// they hold. Wait out GateSettleTime first.
		return &CashbackResult{CampaignID: c.ID, Reason: cashback.Unavailable}, afterPay{}, nil
	}
	return lockedAward(ctx, q, p, paymentID, now)
}

// lockedAward awards under the campaign row lock: an award moves money from
// the campaign account to the user's cash back account as a pair of
// transactions, linked to the payment by a cashback_awards row. When the
// budget reaches zero the campaign ends.
//
// Every awarding payment in a currency waits for the one campaign row, so
// the time between locking it and committing limits payment throughput.
// It is two statements: LockActiveCampaign reads everything the award
// needs, and RecordAward writes it all. Keep it that way.
func lockedAward(ctx context.Context, q *db.Queries, p PayParams, paymentID string, now time.Time) (*CashbackResult, afterPay, error) {
	campaign, err := q.LockActiveCampaign(ctx, db.LockActiveCampaignParams{
		Currency: p.Currency, At: now, UserID: p.UserID, FallbackAfterSecs: int32(GateSettleTime / time.Second),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, afterPay{}, nil
	}
	if err != nil {
		return nil, afterPay{}, fmt.Errorf("find campaign: %w", err)
	}
	r := rules(campaign.RateBps, campaign.MinSpendMinor, campaign.DailyCapMinor, campaign.AwardIncrementMinor)

	// The gate may have changed while this waited for the lock (a switch or
	// a recovery holds it too); the locked row is current.
	if campaign.BudgetGate == GateRedis && !campaign.GateDegraded {
		cb, res, err := reserveCashback(ctx, q, p.Gate, awardInput{
			campaignID:        campaign.ID,
			rules:             r,
			awardedToday:      campaign.AwardedTodayMinor,
			campaignDay:       campaign.CampaignDay,
			cashbackAccountID: campaign.CashbackAccountID,
		}, p, paymentID, now)
		return cb, afterPay{reserved: res}, err
	}
	if campaign.BudgetGate == GateRedis && !campaign.FallbackReady {
		return &CashbackResult{CampaignID: campaign.ID, Reason: cashback.Unavailable}, afterPay{}, nil
	}

	result := &CashbackResult{CampaignID: campaign.ID}
	amount, reason := cashback.Award(p.AmountMinor, r,
		cashback.State{AwardedToday: campaign.AwardedTodayMinor, BudgetRemaining: campaign.BudgetMinor})
	result.Reason = reason
	if amount == 0 {
		return result, afterPay{}, nil
	}
	result.AmountMinor = amount

	if campaign.CashbackAccountID == nil {
		return nil, afterPay{}, errors.New("no cash back account")
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
		return nil, afterPay{}, fmt.Errorf("record award: %w", err)
	}
	result.CampaignEnded = award.CampaignEnded

	var after afterPay
	if campaign.BudgetGate == GateShadow {
		after.shadow = &shadowAward{campaignID: campaign.ID, paymentID: paymentID, amount: amount, increment: r.AwardIncrement}
	}
	return result, after, nil
}

func rules(rateBps int32, minSpend, dailyCap, increment int64) cashback.Rules {
	return cashback.Rules{RateBps: int64(rateBps), MinSpend: minSpend, DailyCap: dailyCap, AwardIncrement: increment}
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
	// (credited, or reserved and still pending) can be reported back.
	cb, err := earlierAward(ctx, q, row.ID)
	if err != nil {
		return PayResult{}, err
	}
	return PayResult{Transaction: row, BalanceMinor: balance, Cashback: cb, Replayed: true}, nil
}

func earlierAward(ctx context.Context, q *db.Queries, paymentID string) (*CashbackResult, error) {
	award, err := q.GetAwardByPayment(ctx, paymentID)
	if err == nil {
		return &CashbackResult{CampaignID: award.CampaignID, AmountMinor: award.AmountMinor, Reason: cashback.Awarded}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read earlier cash back: %w", err)
	}
	pending, err := q.GetPendingByPayment(ctx, paymentID)
	if err == nil {
		return &CashbackResult{CampaignID: pending.CampaignID, AmountMinor: pending.AmountMinor, Reason: cashback.Awarded, Pending: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read earlier pending cash back: %w", err)
	}
	return nil, nil
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "transactions_account_idempotency_key"
}
