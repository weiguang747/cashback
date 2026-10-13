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
	// ErrInsufficientFunds: the account balance is lower than the amount.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrKeyReused: the idempotency key already paid for a different request.
	ErrKeyReused = errors.New("idempotency key already used for a different payment")
)

type PayParams struct {
	AccountID      string
	Currency       string
	AmountMinor    int64 // positive; debited from the account
	MerchantName   string
	IdempotencyKey string
}

type PayResult struct {
	Transaction  db.GetTransactionByIdempotencyKeyRow
	BalanceMinor int64 // the account's balance after the payment
	// Replayed is true when the key had already paid and nothing new was
	// charged (Redis lost the key; the database caught the retry).
	Replayed bool
}

// Pay debits the account and records the payment in one database
// transaction, so the stored balance always equals the sum of transactions.
//
// The conditional debit refuses to overdraw, and its row lock makes
// concurrent payments from one account run in turn. The unique
// (account_id, idempotency_key) constraint means a key can only ever pay
// once, even if the Redis idempotency layer is bypassed.
func Pay(ctx context.Context, pool *pgxpool.Pool, p PayParams) (PayResult, error) {
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
	_, err = q.InsertTransaction(ctx, db.InsertTransactionParams{
		AccountID:      p.AccountID,
		Name:           p.MerchantName,
		Description:    "Payment",
		Kind:           "out",
		AmountMinor:    -p.AmountMinor,
		Currency:       p.Currency,
		CreatedAt:      time.Now(),
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

	row, err := q.GetTransactionByIdempotencyKey(ctx, db.GetTransactionByIdempotencyKeyParams{AccountID: p.AccountID, IdempotencyKey: &key})
	if err != nil {
		return PayResult{}, fmt.Errorf("read payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PayResult{}, fmt.Errorf("commit payment: %w", err)
	}
	return PayResult{Transaction: row, BalanceMinor: balance}, nil
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
	return PayResult{Transaction: row, BalanceMinor: balance, Replayed: true}, nil
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "transactions_account_idempotency_key"
}
