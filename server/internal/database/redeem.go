package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/db"
)

var (
	// ErrNoCashbackAccount: the user has no cash back account in that currency.
	ErrNoCashbackAccount = errors.New("no cash back account")
)

type RedeemParams struct {
	UserID         string
	Currency       string
	AmountMinor    int64 // positive; moved from cash back to main
	IdempotencyKey string
}

type RedeemResult struct {
	// The credit on the main account.
	Transaction          db.GetTransactionByIdempotencyKeyRow
	MainBalanceMinor     int64
	CashbackBalanceMinor int64
	// CashbackPendingMinor is reserved for the user but not yet credited, so
	// not redeemable yet.
	CashbackPendingMinor int64
	MinorUnit            int16
	Replayed             bool
}

// Redeem moves cash back to the user's main account: a debit on the cash
// back account and a credit on the main account, in one database
// transaction. It refuses to redeem more than the cash back balance
// (ErrInsufficientFunds).
//
// Both rows carry the idempotency key, so the unique (account_id,
// idempotency_key) constraint makes a key redeem at most once even if Redis
// loses it. Accounts are locked main first, then cash back, the same order
// payments use, so the two can't deadlock.
func Redeem(ctx context.Context, pool *pgxpool.Pool, p RedeemParams) (RedeemResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return RedeemResult{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	// One round trip finds and locks both accounts, main first.
	accounts, err := q.LockUserAccounts(ctx, db.LockUserAccountsParams{UserID: p.UserID, Currency: p.Currency})
	if err != nil {
		return RedeemResult{}, fmt.Errorf("lock accounts: %w", err)
	}
	var main, cashback *db.LockUserAccountsRow
	for i := range accounts {
		switch accounts[i].Kind {
		case "main":
			main = &accounts[i]
		case "cashback":
			cashback = &accounts[i]
		}
	}
	if main == nil {
		return RedeemResult{}, fmt.Errorf("main account: %w", pgx.ErrNoRows)
	}
	if cashback == nil {
		return RedeemResult{}, ErrNoCashbackAccount
	}

	debit, err := q.DebitAccount(ctx, db.DebitAccountParams{ID: cashback.ID, AmountMinor: p.AmountMinor})
	if errors.Is(err, pgx.ErrNoRows) {
		return RedeemResult{}, ErrInsufficientFunds
	}
	if err != nil {
		return RedeemResult{}, fmt.Errorf("debit cash back: %w", err)
	}

	now := debit.DebitedAt
	key := p.IdempotencyKey
	_, err = q.InsertTransaction(ctx, db.InsertTransactionParams{
		AccountID: cashback.ID, Name: "Cash back redeemed", Description: "To available balance",
		Kind: "out", AmountMinor: -p.AmountMinor, Currency: p.Currency, CreatedAt: now, IdempotencyKey: &key,
	})
	if isIdempotencyConflict(err) {
		tx.Rollback(ctx)
		return replayRedeem(ctx, pool, p, main.ID, cashback.ID, cashback.MinorUnit)
	}
	if err != nil {
		return RedeemResult{}, fmt.Errorf("record cash back debit: %w", err)
	}

	mainBalance, err := q.AddToBalance(ctx, db.AddToBalanceParams{ID: main.ID, DeltaMinor: p.AmountMinor})
	if err != nil {
		return RedeemResult{}, fmt.Errorf("credit main: %w", err)
	}
	credit := db.InsertTransactionParams{
		AccountID: main.ID, Name: "Cash back redeemed", Description: "To available balance",
		Kind: "cashback", AmountMinor: p.AmountMinor, Currency: p.Currency, CreatedAt: now, IdempotencyKey: &key,
	}
	creditID, err := q.InsertTransaction(ctx, credit)
	if isIdempotencyConflict(err) {
		// The key already paid for something on the main account.
		return RedeemResult{}, ErrKeyReused
	}
	if err != nil {
		return RedeemResult{}, fmt.Errorf("record main credit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return RedeemResult{}, fmt.Errorf("commit redemption: %w", err)
	}
	// Built from what was inserted rather than read back: one round trip fewer.
	row := db.GetTransactionByIdempotencyKeyRow{
		ID: creditID, Name: credit.Name, Description: credit.Description, Kind: credit.Kind,
		AmountMinor: credit.AmountMinor, Currency: credit.Currency, MinorUnit: cashback.MinorUnit, CreatedAt: now,
	}
	return RedeemResult{
		Transaction: row, MainBalanceMinor: mainBalance, CashbackBalanceMinor: debit.BalanceMinor,
		CashbackPendingMinor: cashback.PendingMinor, MinorUnit: cashback.MinorUnit,
	}, nil
}

// replayRedeem returns what an idempotency key already redeemed, with the
// accounts' current balances.
func replayRedeem(ctx context.Context, pool *pgxpool.Pool, p RedeemParams, mainID, cashbackID string, minorUnit int16) (RedeemResult, error) {
	q := db.New(pool)
	key := p.IdempotencyKey
	row, err := q.GetTransactionByIdempotencyKey(ctx, db.GetTransactionByIdempotencyKeyParams{AccountID: mainID, IdempotencyKey: &key})
	if errors.Is(err, pgx.ErrNoRows) {
		// The key was used on the cash back account for something else.
		return RedeemResult{}, ErrKeyReused
	}
	if err != nil {
		return RedeemResult{}, fmt.Errorf("read earlier redemption: %w", err)
	}
	if row.AmountMinor != p.AmountMinor || row.Kind != "cashback" {
		return RedeemResult{}, ErrKeyReused
	}
	mainBalance, err := q.GetAccountBalance(ctx, mainID)
	if err != nil {
		return RedeemResult{}, err
	}
	cashbackBalance, err := q.GetAccountBalance(ctx, cashbackID)
	if err != nil {
		return RedeemResult{}, err
	}
	pending, err := q.PendingForAccount(ctx, cashbackID)
	if err != nil {
		return RedeemResult{}, err
	}
	return RedeemResult{
		Transaction: row, MainBalanceMinor: mainBalance, CashbackBalanceMinor: cashbackBalance,
		CashbackPendingMinor: pending, MinorUnit: minorUnit, Replayed: true,
	}, nil
}
