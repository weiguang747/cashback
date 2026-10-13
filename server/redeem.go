package main

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
	"cashback/server/internal/httpx"
	"cashback/server/internal/idempotency"
	"cashback/server/internal/logx"
	"cashback/server/internal/money"
)

type redeemRequest struct {
	// What the user saw and agreed to move, normally the whole balance.
	Amount struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

type redemption struct {
	Transaction     transaction `json:"transaction"`     // the credit on the main account
	Balance         money.Money `json:"balance"`         // main account after redeeming
	CashbackBalance money.Money `json:"cashbackBalance"` // what's left to redeem
}

type redeemResponse struct {
	Redemption redemption `json:"redemption"`
}

// redeemCashback moves cash back into the signed-in user's main account.
// Like payments, it needs an Idempotency-Key and never moves money twice
// for one key.
func redeemCashback(s store, idem idempotency.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		user := currentUser(c)

		key, ok := idempotencyKey(c)
		if !ok {
			return
		}
		var req redeemRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid redemption")
			return
		}
		cur := req.Amount.Currency
		if cur == "" {
			cur = defaultCurrency
		}
		account, err := s.GetUserAccount(ctx, db.GetUserAccountParams{UserID: user.ID, Kind: "cashback", Currency: cur})
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.ErrorCode(c, http.StatusUnprocessableEntity, "unsupported_currency", "you don't have cash back in this currency")
			return
		}
		if err != nil {
			logx.From(c.Request.Context()).Error("get cash back account failed", logx.Err(err))
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}
		amount, err := money.Parse(req.Amount.Value, int(account.MinorUnit))
		if err != nil {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_amount", err.Error())
			return
		}

		fp := fingerprint("redeem", req.Amount.Value, cur)
		runIdempotent(c, idem, key, fp, func() (outcome, error) {
			result, err := s.Redeem(ctx, database.RedeemParams{
				UserID: user.ID, Currency: cur, AmountMinor: amount, IdempotencyKey: key,
			})
			log := logx.From(c.Request.Context()).With("amount_minor", amount, "currency", cur)
			switch {
			case errors.Is(err, database.ErrInsufficientFunds):
				log.Info("redemption declined", "reason", "insufficient_funds")
				return outcome{Status: http.StatusUnprocessableEntity, Body: httpx.ErrorResponse{
					Code: "insufficient_funds", Error: "you don't have that much cash back to redeem"}}, nil
			case errors.Is(err, database.ErrKeyReused):
				log.Warn("redemption declined", "reason", "idempotency_key_reused")
				return keyReused, nil
			case err != nil:
				return outcome{}, err
			}
			log.Info("redemption completed", "transaction_id", result.Transaction.ID, "replayed", result.Replayed)

			m := func(minor int64) money.Money {
				return money.Money{Minor: minor, Currency: cur, Exponent: int(result.MinorUnit)}
			}
			return outcome{Status: http.StatusCreated, Replayed: result.Replayed, Body: redeemResponse{Redemption: redemption{
				Transaction:     toTransaction(txRow(result.Transaction)),
				Balance:         m(result.MainBalanceMinor),
				CashbackBalance: m(result.CashbackBalanceMinor),
			}}}, nil
		})
	}
}
