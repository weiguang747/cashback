package main

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/httpx"
	"cashback/server/internal/logx"
)

type signInRequest struct {
	Email string `json:"email"`
}

type signInResponse struct {
	User user `json:"user"`
}

// signIn looks up the user with this email. This is a demo app, so there
// is no password and no session token: the app sends the returned id as
// X-User-ID (see requireUser).
func signIn(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req signInRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid email")
			return
		}

		email := strings.ToLower(strings.TrimSpace(req.Email))
		if !isValidEmail(email) {
			httpx.Error(c, http.StatusBadRequest, "invalid email")
			return
		}

		ctx := c.Request.Context()
		row, err := s.GetUserByEmail(ctx, email)
		if errors.Is(err, pgx.ErrNoRows) {
			// Masked by logx: enough to spot a typo or repeated guessing.
			logx.From(ctx).Info("sign in rejected", "reason", "unknown_email", "email", email)
			httpx.Error(c, http.StatusUnauthorized, "no account for this email")
			return
		}
		if err != nil {
			logx.From(ctx).Error("get user by email failed", logx.Err(err))
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}
		logx.From(ctx).Info("signed in", "user_id", row.ID)

		c.JSON(http.StatusOK, signInResponse{User: user{ID: row.ID, Email: row.Email, Name: row.Name}})
	}
}

// isValidEmail accepts a bare address like "you@example.com" whose domain has a dot.
func isValidEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return false
	}
	domain := email[strings.LastIndex(email, "@")+1:]
	return strings.Contains(domain, ".")
}
