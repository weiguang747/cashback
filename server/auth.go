package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin"
)

type signInRequest struct {
	Email string `json:"email"`
}

type user struct {
	Email string `json:"email"`
}

type signInResponse struct {
	Token string `json:"token"`
	User  user   `json:"user"`
}

// signIn accepts any valid email for now. The token is random and not
// stored or checked anywhere yet — real authentication comes later.
func signIn(c *gin.Context) {
	var req signInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !isValidEmail(email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}

	token, err := newToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign in"})
		return
	}

	c.JSON(http.StatusOK, signInResponse{Token: token, User: user{Email: email}})
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

// newToken returns a random 32-byte hex string.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
