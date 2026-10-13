package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
	"cashback/server/internal/dbtrace"
	"cashback/server/internal/httpx"
	"cashback/server/internal/money"
)

// store is the part of the database the handlers need. The sqlc
// *db.Queries implements it; tests use a fake.
type store interface {
	GetUser(ctx context.Context, id string) (db.GetUserRow, error)
	GetUserByEmail(ctx context.Context, email string) (db.GetUserByEmailRow, error)
	ListAccounts(ctx context.Context, userID string) ([]db.ListAccountsRow, error)
	ListTransactions(ctx context.Context, arg db.ListTransactionsParams) ([]db.ListTransactionsRow, error)
	SearchTransactions(ctx context.Context, arg db.SearchTransactionsParams) ([]db.SearchTransactionsRow, error)
	ListCashbackTransactions(ctx context.Context, arg db.ListCashbackTransactionsParams) ([]db.ListCashbackTransactionsRow, error)
	GetUserAccount(ctx context.Context, arg db.GetUserAccountParams) (db.GetUserAccountRow, error)
	GetCampaignStatus(ctx context.Context, arg db.GetCampaignStatusParams) (db.GetCampaignStatusRow, error)
	GetMainAccount(ctx context.Context, arg db.GetMainAccountParams) (db.GetMainAccountRow, error)
	Pay(ctx context.Context, p database.PayParams) (database.PayResult, error)
	Redeem(ctx context.Context, p database.RedeemParams) (database.RedeemResult, error)
}

// pgStore is the real store: the sqlc queries plus Pay and Redeem, which
// need their own database transactions.
type pgStore struct {
	*db.Queries
	pool *pgxpool.Pool
	// money limits how many Pay and Redeem transactions hold a pool
	// connection at once. Awarding payments queue on the campaign row lock;
	// without a limit they hold every connection while they wait, and reads
	// like /v1/accounts wait behind them. Extra ones wait here instead,
	// without a connection.
	money slots
}

func newPgStore(pool *pgxpool.Pool) pgStore {
	return pgStore{Queries: db.New(pool), pool: pool, money: newSlots(moneySlots(pool.Config().MaxConns))}
}

// moneySlots leaves at least half the pool to everything else.
func moneySlots(maxConns int32) int {
	return max(1, int(maxConns)/2)
}

// SearchTransactions runs with a plan made for this search text: the
// trigram index for a rare term, the date-ordered index for a short or
// common one. A cached generic plan would always pick one of them, and the
// wrong one costs 100 ms or more on a long history. Planning each search
// costs about 0.1 ms.
func (s pgStore) SearchTransactions(ctx context.Context, arg db.SearchTransactionsParams) ([]db.SearchTransactionsRow, error) {
	return db.New(freshPlans{s.pool}).SearchTransactions(ctx, arg)
}

// freshPlans runs every query as an unnamed statement (pgx's
// QueryExecModeExec), which Postgres plans with the actual parameters each
// time instead of caching a plan.
type freshPlans struct{ pool *pgxpool.Pool }

func (f freshPlans) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return f.pool.Exec(ctx, sql, append([]any{pgx.QueryExecModeExec}, args...)...)
}

func (f freshPlans) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return f.pool.Query(ctx, sql, append([]any{pgx.QueryExecModeExec}, args...)...)
}

func (f freshPlans) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f.pool.QueryRow(ctx, sql, append([]any{pgx.QueryExecModeExec}, args...)...)
}

func (s pgStore) Pay(ctx context.Context, p database.PayParams) (database.PayResult, error) {
	if err := s.money.acquire(ctx); err != nil {
		return database.PayResult{}, err
	}
	defer s.money.release()
	return database.Pay(ctx, s.pool, p)
}

func (s pgStore) Redeem(ctx context.Context, p database.RedeemParams) (database.RedeemResult, error) {
	if err := s.money.acquire(ctx); err != nil {
		return database.RedeemResult{}, err
	}
	defer s.money.release()
	return database.Redeem(ctx, s.pool, p)
}

// slots is a counting semaphore whose wait ends with the request.
type slots chan struct{}

func newSlots(n int) slots { return make(slots, n) }

// acquire waits for a free slot, or returns the context's error if the
// request times out or the client leaves first (nothing has run yet, so
// the handler's usual timeout and cancel handling applies).
func (s slots) acquire(ctx context.Context) error {
	start := time.Now()
	defer func() { dbtrace.AddWait(ctx, time.Since(start)) }()
	select {
	case s <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("waiting for a database slot: %w", ctx.Err())
	}
}

func (s slots) release() { <-s }

type transaction struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Amount      money.Money `json:"amount"` // {"value":"-35000.00","currency":"IDR"}; negative = money out
	Kind        string      `json:"kind"`   // "in", "out" or "cashback"
	CreatedAt   time.Time   `json:"createdAt"`
	// Cash back this payment earned; omitted when none.
	Cashback *money.Money `json:"cashback,omitempty"`
}

type transactionsResponse struct {
	Transactions []transaction `json:"transactions"`
	// Pass as ?cursor= for the next page; null on the last page.
	NextCursor *string `json:"nextCursor"`
}

// maxSearchLen bounds ?q=, in characters.
const maxSearchLen = 100

// listTransactions returns one page of the signed-in user's main-account
// transactions, newest first. Query params, all optional: limit and cursor
// (see paging.go), kind ("in", "out" or "cashback") and q (case-insensitive
// text in the name).
func listTransactions(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := pageParams(c)
		if !ok {
			return
		}
		params := db.ListTransactionsParams{
			UserID: currentUser(c).ID, Limit: p.limit, BeforeCreatedAt: p.beforeCreatedAt, BeforeID: p.beforeID,
		}
		if k := c.Query("kind"); k != "" {
			if k != "in" && k != "out" && k != "cashback" {
				httpx.Error(c, http.StatusBadRequest, "kind must be in, out or cashback")
				return
			}
			params.Kind = &k
		}
		q := strings.TrimSpace(c.Query("q"))
		if utf8.RuneCountInString(q) > maxSearchLen {
			httpx.Error(c, http.StatusBadRequest, fmt.Sprintf("search must be at most %d characters", maxSearchLen))
			return
		}

		var rows []db.ListTransactionsRow
		var err error
		if q == "" {
			rows, err = s.ListTransactions(c.Request.Context(), params)
		} else {
			// A separate query, planned for this search text (see pgStore.SearchTransactions).
			var found []db.SearchTransactionsRow
			found, err = s.SearchTransactions(c.Request.Context(), db.SearchTransactionsParams{
				UserID: params.UserID, Limit: params.Limit, BeforeCreatedAt: params.BeforeCreatedAt, BeforeID: params.BeforeID,
				Kind: params.Kind, NamePattern: "%" + likeEscaper.Replace(q) + "%",
			})
			for _, r := range found {
				rows = append(rows, db.ListTransactionsRow(r))
			}
		}
		if err != nil {
			httpx.InternalError(c, "list transactions failed", err)
			return
		}
		rows, next := trim(p, rows, func(r db.ListTransactionsRow) (time.Time, string) { return r.CreatedAt, r.ID })

		txns := make([]transaction, 0, len(rows))
		for _, r := range rows {
			t := toTransaction(txRow{r.ID, r.Name, r.Description, r.Kind, r.AmountMinor, r.Currency, r.MinorUnit, r.CreatedAt})
			if r.CashbackMinor != nil {
				t.Cashback = &money.Money{Minor: *r.CashbackMinor, Currency: r.Currency, Exponent: int(r.MinorUnit)}
			}
			txns = append(txns, t)
		}
		c.JSON(http.StatusOK, transactionsResponse{Transactions: txns, NextCursor: next})
	}
}

// likeEscaper makes user text match literally inside an ILIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// txRow is the shape every transaction query shares.
type txRow struct {
	ID          string
	Name        string
	Description string
	Kind        string
	AmountMinor int64
	Currency    string
	MinorUnit   int16
	CreatedAt   time.Time
}

func toTransaction(r txRow) transaction {
	return transaction{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Amount:      money.Money{Minor: r.AmountMinor, Currency: r.Currency, Exponent: int(r.MinorUnit)},
		Kind:        r.Kind,
		// Always UTC on the wire; the app shows it in the device's time zone.
		CreatedAt: r.CreatedAt.UTC(),
	}
}
