package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/db"
)

// fakeStore serves fixed rows, newest first, the way the real query does.
type fakeStore struct {
	rows []db.ListTransactionsRow
	err  error
}

func (f fakeStore) ListTransactions(_ context.Context, limit *int32) ([]db.ListTransactionsRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit != nil && int(*limit) < len(f.rows) {
		return f.rows[:*limit], nil
	}
	return f.rows, nil
}

var wib = time.FixedZone("WIB", 7*60*60)

func testRows(n int) []db.ListTransactionsRow {
	rows := make([]db.ListTransactionsRow, n)
	for i := range rows {
		rows[i] = db.ListTransactionsRow{
			ID: "t" + string(rune('a'+i)), Name: "Kopi Kenangan", Description: "Payment", Kind: "out",
			AmountMinor: -3_500_000, Currency: "IDR", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 13, 8, 42-i, 0, 0, wib),
		}
	}
	return rows
}

func TestListTransactions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(fakeStore{rows: testRows(12)})

	tests := []struct {
		name      string
		query     string
		wantCode  int
		wantCount int
	}{
		{"all", "", http.StatusOK, 12},
		{"limit", "?limit=6", http.StatusOK, 6},
		{"limit larger than total", "?limit=100", http.StatusOK, 12},
		{"zero limit", "?limit=0", http.StatusBadRequest, 0},
		{"negative limit", "?limit=-1", http.StatusBadRequest, 0},
		{"non-numeric limit", "?limit=abc", http.StatusBadRequest, 0},
		{"limit over int32", "?limit=9999999999", http.StatusBadRequest, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/transactions"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

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
	rows := []db.ListTransactionsRow{
		{ID: "a", Name: "Kopi Kenangan", Description: "Payment", Kind: "out", AmountMinor: -3_500_000, Currency: "IDR", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 13, 8, 42, 0, 0, wib)},
		{ID: "b", Name: "Big", Description: "Received", Kind: "in", AmountMinor: 9_007_199_254_740_993, Currency: "USD", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 12, 8, 0, 0, 0, wib)},
		{ID: "c", Name: "Uniqlo", Description: "Payment", Kind: "out", AmountMinor: -3_990, Currency: "JPY", MinorUnit: 0,
			CreatedAt: time.Date(2026, 10, 11, 8, 0, 0, 0, wib)},
	}
	router := setupRouter(fakeStore{rows: rows})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/transactions", nil))
	body := w.Body.String()

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
	router := setupRouter(fakeStore{err: errors.New("db down")})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/transactions", nil))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "db down") {
		t.Errorf("status = %d body = %s, want 500 without internal details", w.Code, w.Body)
	}
}
