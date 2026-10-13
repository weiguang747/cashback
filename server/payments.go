package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
	"cashback/server/internal/httpx"
	"cashback/server/internal/idempotency"
	"cashback/server/internal/logx"
	"cashback/server/internal/money"
)

type paymentRequest struct {
	MerchantID   string `json:"merchantId"`
	MerchantName string `json:"merchantName"`
	Amount       struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

type payment struct {
	Transaction transaction `json:"transaction"`
	Balance     money.Money `json:"balance"` // main account balance after paying
	// Null when no campaign was live; otherwise what was earned and why.
	Cashback *paymentCashback `json:"cashback"`
}

type paymentCashback struct {
	Amount money.Money `json:"amount"`
	// "awarded", "below_minimum", "daily_cap_reached" or "budget_exhausted".
	Reason string `json:"reason"`
}

type paymentResponse struct {
	Payment payment `json:"payment"`
}

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

const maxMerchantLen = 100

// createPayment debits the signed-in user's main account and records the
// payment. Clients must send an Idempotency-Key header (a fresh UUID per
// payment, reused on retries): repeating a request with the same key
// returns the first response and never charges twice.
func createPayment(s store, idem idempotency.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		user := currentUser(c)

		key, ok := idempotencyKey(c)
		if !ok {
			return
		}

		var req paymentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid payment")
			return
		}
		req.MerchantID = strings.TrimSpace(req.MerchantID)
		req.MerchantName = strings.TrimSpace(req.MerchantName)
		if req.MerchantID == "" || req.MerchantName == "" {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_merchant", "merchant is required")
			return
		}
		if utf8.RuneCountInString(req.MerchantID) > maxMerchantLen || utf8.RuneCountInString(req.MerchantName) > maxMerchantLen {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_merchant",
				fmt.Sprintf("merchant id and name must be at most %d characters", maxMerchantLen))
			return
		}

		account, err := s.GetMainAccount(ctx, db.GetMainAccountParams{UserID: user.ID, Currency: req.Amount.Currency})
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.ErrorCode(c, http.StatusUnprocessableEntity, "unsupported_currency", "you don't have an account in this currency")
			return
		}
		if err != nil {
			httpx.InternalError(c, "get main account failed", err)
			return
		}
		amount, err := money.Parse(req.Amount.Value, int(account.MinorUnit))
		if err != nil {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_amount", err.Error())
			return
		}

		fp := fingerprint("payment", req.MerchantID, req.MerchantName, req.Amount.Value, req.Amount.Currency)
		runIdempotent(c, idem, key, fp, func() (outcome, error) {
			result, err := s.Pay(ctx, database.PayParams{
				UserID:         user.ID,
				AccountID:      account.ID,
				Currency:       account.Currency,
				AmountMinor:    amount,
				MerchantName:   req.MerchantName,
				IdempotencyKey: key,
			})
			log := logx.From(c.Request.Context()).With("merchant_id", req.MerchantID, "amount_minor", amount, "currency", account.Currency)
			switch {
			case errors.Is(err, database.ErrInsufficientFunds):
				log.Info("payment declined", "reason", "insufficient_funds")
				return outcome{Status: http.StatusUnprocessableEntity, Body: httpx.ErrorResponse{
					Code: "insufficient_funds", Error: "you don't have enough balance for this payment"}}, nil
			case errors.Is(err, database.ErrKeyReused):
				log.Warn("payment declined", "reason", "idempotency_key_reused")
				return keyReused, nil
			case err != nil:
				return outcome{}, err
			}

			// Balances are deliberately not logged; ids and amounts are enough
			// to trace a payment, and the database has the rest.
			attrs := []any{"transaction_id", result.Transaction.ID, "replayed", result.Replayed}
			if cb := result.Cashback; cb != nil {
				attrs = append(attrs, "campaign_id", cb.CampaignID, "cashback_minor", cb.AmountMinor, "cashback_reason", string(cb.Reason))
				if cb.CampaignEnded {
					log.Info("campaign ended: budget used up", "campaign_id", cb.CampaignID)
				}
			}
			log.Info("payment completed", attrs...)

			m := func(minor int64) money.Money {
				return money.Money{Minor: minor, Currency: account.Currency, Exponent: int(account.MinorUnit)}
			}
			txn := toTransaction(txRow(result.Transaction))
			var cb *paymentCashback
			if result.Cashback != nil {
				earned := m(result.Cashback.AmountMinor)
				cb = &paymentCashback{Amount: earned, Reason: string(result.Cashback.Reason)}
				if result.Cashback.AmountMinor > 0 {
					txn.Cashback = &earned
				}
			}
			return outcome{Status: http.StatusCreated, Replayed: result.Replayed, Body: paymentResponse{Payment: payment{
				Transaction: txn,
				Balance:     m(result.BalanceMinor),
				Cashback:    cb,
			}}}, nil
		})
	}
}

// fingerprint identifies the request body, so a reused key with a
// different payment is caught instead of replaying the wrong response.
func fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
