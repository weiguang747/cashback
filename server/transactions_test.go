package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/db"
)

func TestListTransactions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := testRouter(demoStore(12))

	tests := []struct {
		name      string
		user      string
		query     string
		wantCode  int
		wantCount int
	}{
		{"all", andiID, "", http.StatusOK, 12},
		{"limit", andiID, "?limit=6", http.StatusOK, 6},
		{"limit larger than total", andiID, "?limit=100", http.StatusOK, 12},
		{"only the signed-in user's rows", sarahID, "", http.StatusOK, 1},
		{"zero limit", andiID, "?limit=0", http.StatusBadRequest, 0},
		{"negative limit", andiID, "?limit=-1", http.StatusBadRequest, 0},
		{"non-numeric limit", andiID, "?limit=abc", http.StatusBadRequest, 0},
		{"limit over int32", andiID, "?limit=9999999999", http.StatusBadRequest, 0},
		{"signed out", "", "", http.StatusUnauthorized, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := get(router, "/v1/transactions"+tt.query, tt.user)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.wantCode, w.Body)
			}
			if tt.wantCode != http.StatusOK {
				return
			}

			var resp struct {
				Transactions []json.RawMessage `json:"transactions"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if len(resp.Transactions) != tt.wantCount {
				t.Errorf("got %d transactions, want %d", len(resp.Transactions), tt.wantCount)
			}
		})
	}
}

// Amounts go out as exact decimal strings with a currency, and times in UTC.
func TestListTransactionsJSONShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(0)
	s.txns[andiID] = []db.ListTransactionsRow{
		{ID: "a", Name: "Kopi Kenangan", Description: "Payment", Kind: "out", AmountMinor: -3_500_000, Currency: "IDR", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 13, 8, 42, 0, 0, wib)},
		{ID: "b", Name: "Big", Description: "Received", Kind: "in", AmountMinor: 9_007_199_254_740_993, Currency: "USD", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 12, 8, 0, 0, 0, wib)},
		{ID: "c", Name: "Uniqlo", Description: "Payment", Kind: "out", AmountMinor: -3_990, Currency: "JPY", MinorUnit: 0,
			CreatedAt: time.Date(2026, 10, 11, 8, 0, 0, 0, wib)},
	}
	body := get(testRouter(s), "/v1/transactions", andiID).Body.String()

	for _, want := range []string{
		`"amount":{"value":"-35000.00","currency":"IDR"}`,
		`"amount":{"value":"90071992547409.93","currency":"USD"}`,
		`"amount":{"value":"-3990","currency":"JPY"}`,
		`"createdAt":"2026-10-13T01:42:00Z"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s\nbody: %s", want, body)
		}
	}
}

func TestListTransactionsStoreError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(1)
	router := testRouter(failingTransactions{s})

	w := get(router, "/v1/transactions", andiID)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "db down") {
		t.Errorf("status = %d body = %s, want 500 without internal details", w.Code, w.Body)
	}
}

// failingTransactions finds users but fails to list their transactions.
type failingTransactions struct{ fakeStore }

func (failingTransactions) ListTransactions(_ context.Context, _ db.ListTransactionsParams) ([]db.ListTransactionsRow, error) {
	return nil, errors.New("db down")
}
