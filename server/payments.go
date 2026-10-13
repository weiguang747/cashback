package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
	"cashback/server/internal/httpx"
	"cashback/server/internal/idempotency"
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

		key := c.GetHeader("Idempotency-Key")
		if !idempotencyKeyPattern.MatchString(key) {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_idempotency_key",
				"Idempotency-Key header must be 8-128 letters, digits, - or _")
			return
		}

		var req paymentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid payment")
			return
		}
		req.MerchantID = strings.TrimSpace(req.MerchantID)
		req.MerchantName = strings.TrimSpace(req.MerchantName)
		if req.MerchantID == "" || req.MerchantName == "" || len(req.MerchantID) > maxMerchantLen || len(req.MerchantName) > maxMerchantLen {
			httpx.Error(c, http.StatusBadRequest, "merchant is required")
			return
		}

		account, err := s.GetMainAccount(ctx, db.GetMainAccountParams{UserID: user.ID, Currency: req.Amount.Currency})
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.ErrorCode(c, http.StatusUnprocessableEntity, "unsupported_currency", "you don't have an account in this currency")
			return
		}
		if err != nil {
			log.Printf("get main account: %v", err)
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}
		amount, err := money.Parse(req.Amount.Value, int(account.MinorUnit))
		if err != nil {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_amount", err.Error())
			return
		}

		fp := fingerprint(req.MerchantID, req.MerchantName, req.Amount.Value, req.Amount.Currency)
		begin, err := idem.Begin(ctx, user.ID, key, fp)
		if err != nil {
			// Fail closed: never pay without the duplicate guard.
			log.Printf("idempotency begin: %v", err)
			httpx.ErrorCode(c, http.StatusServiceUnavailable, "unavailable", "payments are unavailable right now, please try again")
			return
		}
		switch begin.Outcome {
		case idempotency.Replay:
			c.Header("Idempotent-Replayed", "true")
			c.Data(begin.Response.Status, gin.MIMEJSON, begin.Response.Body)
			return
		case idempotency.InProgress:
			httpx.ErrorCode(c, http.StatusConflict, "payment_in_progress", "this payment is already being processed")
			return
		case idempotency.Mismatch:
			httpx.ErrorCode(c, http.StatusUnprocessableEntity, "idempotency_key_reused", "this Idempotency-Key was already used for a different payment")
			return
		}

		// From here on, record the outcome even if the client disconnects.
		bg := context.WithoutCancel(ctx)

		result, err := s.Pay(ctx, database.PayParams{
			UserID:         user.ID,
			AccountID:      account.ID,
			Currency:       account.Currency,
			AmountMinor:    amount,
			MerchantName:   req.MerchantName,
			IdempotencyKey: key,
		})

		var status int
		var body any
		switch {
		case errors.Is(err, database.ErrInsufficientFunds):
			status = http.StatusUnprocessableEntity
			body = httpx.ErrorResponse{Code: "insufficient_funds", Error: "you don't have enough balance for this payment"}
		case errors.Is(err, database.ErrKeyReused):
			status = http.StatusUnprocessableEntity
			body = httpx.ErrorResponse{Code: "idempotency_key_reused", Error: "this Idempotency-Key was already used for a different payment"}
		case err != nil:
			// Unexpected: let the client retry with the same key.
			log.Printf("pay: %v", err)
			if err := idem.Release(bg, user.ID, key); err != nil {
				log.Printf("idempotency release: %v", err)
			}
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		default:
			status = http.StatusCreated
			txn := toTransaction(txRow(result.Transaction))
			var cb *paymentCashback
			if result.Cashback != nil {
				earned := money.Money{Minor: result.Cashback.AmountMinor, Currency: account.Currency, Exponent: int(account.MinorUnit)}
				cb = &paymentCashback{Amount: earned, Reason: string(result.Cashback.Reason)}
				if result.Cashback.AmountMinor > 0 {
					txn.Cashback = &earned
				}
			}
			body = paymentResponse{Payment: payment{
				Transaction: txn,
				Balance:     money.Money{Minor: result.BalanceMinor, Currency: account.Currency, Exponent: int(account.MinorUnit)},
				Cashback:    cb,
			}}
		}

		raw, err := json.Marshal(body)
		if err != nil {
			log.Printf("encode payment response: %v", err)
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}
		// A failed cache write is only logged: the database's unique key
		// still stops a retry from charging twice.
		if err := idem.Complete(bg, user.ID, key, fp, idempotency.Response{Status: status, Body: raw}); err != nil {
			log.Printf("idempotency complete: %v", err)
		}
		if result.Replayed {
			c.Header("Idempotent-Replayed", "true")
		}
		c.Data(status, gin.MIMEJSON, raw)
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
