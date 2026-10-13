package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(demoStore(1))

	tests := []struct {
		name     string
		userID   string
		wantCode int
	}{
		{"known user", andiID, http.StatusOK},
		{"no header", "", http.StatusUnauthorized},
		{"not a UUID", "andi", http.StatusUnauthorized},
		{"unknown user", "33333333-3333-4333-8333-333333333333", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, path := range []string{"/v1/accounts", "/v1/transactions"} {
				w := get(router, path, tt.userID)
				if w.Code != tt.wantCode {
					t.Errorf("%s: status = %d, want %d (body: %s)", path, w.Code, tt.wantCode, w.Body)
				}
				if tt.wantCode == http.StatusUnauthorized && !strings.Contains(w.Body.String(), `"sign in required"`) {
					t.Errorf("%s: body = %s, want sign in required", path, w.Body)
				}
			}
		})
	}
}

func TestRequireUserStoreError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(fakeStore{err: errors.New("db down")})

	w := get(router, "/v1/accounts", andiID)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (a database error is not a sign-in problem)", w.Code)
	}
}

func TestListAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(demoStore(0))

	w := get(router, "/v1/accounts", andiID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	want := `{"accounts":[` +
		`{"id":"acc-main","kind":"main","balance":{"value":"12450000.00","currency":"IDR"}},` +
		`{"id":"acc-cb","kind":"cashback","balance":{"value":"0.00","currency":"IDR"}}]}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %s\nwant   %s", got, want)
	}

	// A user without accounts gets an empty list, not null.
	if body := get(router, "/v1/accounts", sarahID).Body.String(); body != `{"accounts":[]}` {
		t.Errorf("sarah body = %s, want empty list", body)
	}
}
