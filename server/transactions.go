package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
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
	ListCashbackTransactions(ctx context.Context, arg db.ListCashbackTransactionsParams) ([]db.ListCashbackTransactionsRow, error)
	GetUserAccount(ctx context.Context, arg db.GetUserAccountParams) (db.GetUserAccountRow, error)
	GetCampaignStatus(ctx context.Context, arg db.GetCampaignStatusParams) (db.GetCampaignStatusRow, error)
	GetMainAccount(ctx context.Context, arg db.GetMainAccountParams) (db.GetMainAccountRow, error)
	Pay(ctx context.Context, p database.PayParams) (database.PayResult, error)
}

// pgStore is the real store: the sqlc queries plus Pay, which needs its
// own database transaction.
type pgStore struct {
	*db.Queries
	pool *pgxpool.Pool
}

func newPgStore(pool *pgxpool.Pool) pgStore { return pgStore{Queries: db.New(pool), pool: pool} }

func (s pgStore) Pay(ctx context.Context, p database.PayParams) (database.PayResult, error) {
	return database.Pay(ctx, s.pool, p)
}

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
}

// listTransactions returns the signed-in user's main-account transactions, newest first.
// Optional query param: limit (positive integer).
func listTransactions(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var limit *int32
		if q := c.Query("limit"); q != "" {
			n, err := strconv.ParseInt(q, 10, 32)
			if err != nil || n < 1 {
				httpx.Error(c, http.StatusBadRequest, "limit must be a positive integer")
				return
			}
			n32 := int32(n)
			limit = &n32
		}

		rows, err := s.ListTransactions(c.Request.Context(), db.ListTransactionsParams{UserID: currentUser(c).ID, Limit: limit})
		if err != nil {
			log.Printf("list transactions: %v", err)
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}

		txns := make([]transaction, 0, len(rows))
		for _, r := range rows {
			t := toTransaction(txRow{r.ID, r.Name, r.Description, r.Kind, r.AmountMinor, r.Currency, r.MinorUnit, r.CreatedAt})
			if r.CashbackMinor != nil {
				t.Cashback = &money.Money{Minor: *r.CashbackMinor, Currency: r.Currency, Exponent: int(r.MinorUnit)}
			}
			txns = append(txns, t)
		}
		c.JSON(http.StatusOK, transactionsResponse{Transactions: txns})
	}
}

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
