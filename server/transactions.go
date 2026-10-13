package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/db"
	"cashback/server/internal/httpx"
	"cashback/server/internal/money"
)

// transactionStore is the part of the database the handlers need. The sqlc
// *db.Queries implements it; tests use a fake.
type transactionStore interface {
	ListTransactions(ctx context.Context, limit *int32) ([]db.ListTransactionsRow, error)
}

type transaction struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Amount      money.Money `json:"amount"` // {"value":"-35000.00","currency":"IDR"}; negative = money out
	Kind        string      `json:"kind"`   // "in", "out" or "cashback"
	CreatedAt   time.Time   `json:"createdAt"`
}

type transactionsResponse struct {
	Transactions []transaction `json:"transactions"`
}

// listTransactions returns transactions newest first.
// Optional query param: limit (positive integer).
func listTransactions(store transactionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var limit *int32
		if s := c.Query("limit"); s != "" {
			n, err := strconv.ParseInt(s, 10, 32)
			if err != nil || n < 1 {
				httpx.Error(c, http.StatusBadRequest, "limit must be a positive integer")
				return
			}
			n32 := int32(n)
			limit = &n32
		}

		rows, err := store.ListTransactions(c.Request.Context(), limit)
		if err != nil {
			log.Printf("list transactions: %v", err)
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}

		txns := make([]transaction, 0, len(rows))
		for _, r := range rows {
			txns = append(txns, transaction{
				ID:          r.ID,
				Name:        r.Name,
				Description: r.Description,
				Amount:      money.Money{Minor: r.AmountMinor, Currency: r.Currency, Exponent: int(r.MinorUnit)},
				Kind:        r.Kind,
				// Always UTC on the wire; the app shows it in the device's time zone.
				CreatedAt: r.CreatedAt.UTC(),
			})
		}
		c.JSON(http.StatusOK, transactionsResponse{Transactions: txns})
	}
}
