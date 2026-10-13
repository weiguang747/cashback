package main

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/httpx"
	"cashback/server/internal/logx"
)

type user struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

const userKey = "user"

// requireUser loads the signed-in user from the X-User-ID header and stops
// the request with 401 if there isn't one.
//
// This is a demo stand-in for auth: anyone who knows a user's id can act as
// them. Replace this one function with a token check when real auth lands.
func requireUser(s store) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.GetHeader("X-User-ID")
		// Check the format first so a bad value can't reach Postgres as a 500.
		// The header is the demo credential, so it's never logged when rejected.
		if _, err := uuid.Parse(id); err != nil {
			reason := "malformed_user_id"
			if id == "" {
				reason = "missing_user_id"
			}
			logx.From(ctx).Warn("auth rejected", "reason", reason)
			httpx.Error(c, http.StatusUnauthorized, "sign in required")
			return
		}

		row, err := s.GetUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			logx.From(ctx).Warn("auth rejected", "reason", "unknown_user")
			httpx.Error(c, http.StatusUnauthorized, "sign in required")
			return
		}
		if err != nil {
			httpx.InternalError(c, "get user failed", err)
			return
		}

		c.Set(userKey, user{ID: row.ID, Email: row.Email, Name: row.Name})
		// Every later log line for this request, including the access log,
		// says whose request it was.
		httpx.AddToLogger(c, "user_id", row.ID)
		c.Next()
	}
}

// currentUser returns the user that requireUser stored on the request.
func currentUser(c *gin.Context) user {
	return c.MustGet(userKey).(user)
}
