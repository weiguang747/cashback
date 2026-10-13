package main

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
	"cashback/server/internal/logx"
	"cashback/server/internal/money"
)

type account struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`    // "main" or "cashback"
	Balance money.Money `json:"balance"` // {"value":"12450000.00","currency":"IDR"}
}

type accountsResponse struct {
	Accounts []account `json:"accounts"`
}

// listAccounts returns the signed-in user's accounts and balances, main first.
func listAccounts(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := s.ListAccounts(c.Request.Context(), currentUser(c).ID)
		if err != nil {
			logx.From(c.Request.Context()).Error("list accounts failed", logx.Err(err))
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}

		accounts := make([]account, 0, len(rows))
		for _, r := range rows {
			accounts = append(accounts, account{
				ID:      r.ID,
				Kind:    r.Kind,
				Balance: money.Money{Minor: r.BalanceMinor, Currency: r.Currency, Exponent: int(r.MinorUnit)},
			})
		}
		c.JSON(http.StatusOK, accountsResponse{Accounts: accounts})
	}
}
