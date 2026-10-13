package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestListTransactions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter()

	tests := []struct {
		name      string
		query     string
		wantCode  int
		wantCount int
	}{
		{"all", "", http.StatusOK, len(demoTransactions)},
		{"limit", "?limit=6", http.StatusOK, 6},
		{"limit larger than total", "?limit=100", http.StatusOK, len(demoTransactions)},
		{"zero limit", "?limit=0", http.StatusBadRequest, 0},
		{"negative limit", "?limit=-1", http.StatusBadRequest, 0},
		{"non-numeric limit", "?limit=abc", http.StatusBadRequest, 0},
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

			var resp transactionsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if len(resp.Transactions) != tt.wantCount {
				t.Errorf("got %d transactions, want %d", len(resp.Transactions), tt.wantCount)
			}
		})
	}
}

func TestBuildDemoTransactionsNewestFirst(t *testing.T) {
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, wib)
	txns := buildDemoTransactions(now)

	for i := 1; i < len(txns); i++ {
		if txns[i].CreatedAt.After(txns[i-1].CreatedAt) {
			t.Fatalf("not sorted newest first at index %d", i)
		}
	}
	if got := txns[0]; got.ID != "t1" || !got.CreatedAt.Equal(time.Date(2026, 10, 13, 8, 42, 0, 0, wib)) {
		t.Errorf("first = %s at %v, want t1 at 2026-10-13 08:42 WIB", got.ID, got.CreatedAt)
	}
}
