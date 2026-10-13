package main

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
)

type signInRequest struct {
	Email string `json:"email"`
}

type user struct {
	Email string `json:"email"`
}

type signInResponse struct {
	User user `json:"user"`
}

// signIn accepts any valid email. This is a demo app, so there is no
// password and no session token.
func signIn(c *gin.Context) {
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

	c.JSON(http.StatusOK, signInResponse{User: user{Email: email}})
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
