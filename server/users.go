package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/httpx"
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
		id := c.GetHeader("X-User-ID")
		// Check the format first so a bad value can't reach Postgres as a 500.
		if _, err := uuid.Parse(id); err != nil {
			httpx.Error(c, http.StatusUnauthorized, "sign in required")
			return
		}

		row, err := s.GetUser(c.Request.Context(), id)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(c, http.StatusUnauthorized, "sign in required")
			return
		}
		if err != nil {
			log.Printf("get user: %v", err)
			httpx.Error(c, http.StatusInternalServerError, "something went wrong")
			return
		}

		c.Set(userKey, user{ID: row.ID, Email: row.Email, Name: row.Name})
		c.Next()
	}
}

// currentUser returns the user that requireUser stored on the request.
func currentUser(c *gin.Context) user {
	return c.MustGet(userKey).(user)
}
