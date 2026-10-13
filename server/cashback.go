package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/db"
	"cashback/server/internal/httpx"
	"cashback/server/internal/logx"
	"cashback/server/internal/money"
)

// Every demo account is in rupiah; ?currency= picks another.
const defaultCurrency = "IDR"

func currencyParam(c *gin.Context) string {
	if cur := c.Query("currency"); cur != "" {
		return cur
	}
	return defaultCurrency
}

type campaignStatus struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Status          string      `json:"status"` // "active" or "ended"
	RateBps         int32       `json:"rateBps"`
	MinSpend        money.Money `json:"minSpend"`
	DailyCap        money.Money `json:"dailyCap"`
	EarnedToday     money.Money `json:"earnedToday"`
	BudgetRemaining money.Money `json:"budgetRemaining"`
	// When the daily cap resets: next midnight in the campaign's time zone.
	ResetAt time.Time `json:"resetAt"`
}

type campaignResponse struct {
	Campaign *campaignStatus `json:"campaign"` // null if there has never been one
}

// getCampaign returns the live cash back campaign (or the latest ended one)
// with the signed-in user's progress against today's cap.
func getCampaign(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		r, err := s.GetCampaignStatus(c.Request.Context(), db.GetCampaignStatusParams{
			At: time.Now(), UserID: currentUser(c).ID, Currency: currencyParam(c),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusOK, campaignResponse{})
			return
		}
		if err != nil {
			logx.From(c.Request.Context()).Error("get campaign failed", logx.Err(err))
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}

		m := func(minor int64) money.Money {
			return money.Money{Minor: minor, Currency: r.Currency, Exponent: int(r.MinorUnit)}
		}
		status := "ended"
		if r.Active {
			status = "active"
		}
		c.JSON(http.StatusOK, campaignResponse{Campaign: &campaignStatus{
			ID:              r.ID,
			Name:            r.Name,
			Status:          status,
			RateBps:         r.RateBps,
			MinSpend:        m(r.MinSpendMinor),
			DailyCap:        m(r.DailyCapMinor),
			EarnedToday:     m(r.EarnedTodayMinor),
			BudgetRemaining: m(r.BudgetRemainingMinor),
			ResetAt:         r.ResetAt.UTC(),
		}})
	}
}

type cashbackHistoryResponse struct {
	Balance      money.Money   `json:"balance"`
	Transactions []transaction `json:"transactions"`
}

// listCashbackHistory returns the signed-in user's cash back balance and
// the activity on their cash back account, newest first.
// Optional query param: limit (positive integer).
func listCashbackHistory(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		user := currentUser(c)
		cur := currencyParam(c)

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

		account, err := s.GetUserAccount(ctx, db.GetUserAccountParams{UserID: user.ID, Kind: "cashback", Currency: cur})
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.ErrorCode(c, http.StatusNotFound, "no_cashback_account", "you don't have a cash back account in this currency")
			return
		}
		if err != nil {
			logx.From(c.Request.Context()).Error("get cash back account failed", logx.Err(err))
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}
		rows, err := s.ListCashbackTransactions(ctx, db.ListCashbackTransactionsParams{UserID: user.ID, Currency: cur, Limit: limit})
		if err != nil {
			logx.From(c.Request.Context()).Error("list cash back transactions failed", logx.Err(err))
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}

		txns := make([]transaction, 0, len(rows))
		for _, r := range rows {
			txns = append(txns, toTransaction(txRow(r)))
		}
		c.JSON(http.StatusOK, cashbackHistoryResponse{
			Balance:      money.Money{Minor: account.BalanceMinor, Currency: cur, Exponent: int(account.MinorUnit)},
			Transactions: txns,
		})
	}
}
