package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func redeem(h http.Handler, userID, key, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/cashback/redeem",
		strings.NewReader(`{"amount":{"value":"`+value+`","currency":"IDR"}}`))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func withCashback(minor int64) fakeStore {
	s := demoStore(0)
	s.accounts[andiID][1].BalanceMinor = minor
	return s
}

func TestRedeemCashback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := withCashback(4_475_000) // Rp 44.750
	router := testRouter(s)

	first := redeem(router, andiID, "redeem-0001", "44750")
	if first.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", first.Code, first.Body)
	}
	for _, want := range []string{
		`"amount":{"value":"44750.00","currency":"IDR"},"kind":"cashback"`,
		`"balance":{"value":"12494750.00","currency":"IDR"}`,
		`"cashbackBalance":{"value":"0.00","currency":"IDR"}`,
	} {
		if !strings.Contains(first.Body.String(), want) {
			t.Errorf("body missing %s\nbody: %s", want, first.Body)
		}
	}

	// A retry with the same key replays, and moves nothing more.
	second := redeem(router, andiID, "redeem-0001", "44750")
	if second.Body.String() != first.Body.String() || second.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("replay = %d %s, want the first response replayed", second.Code, second.Body)
	}
	if main, cb := s.accounts[andiID][0].BalanceMinor, s.accounts[andiID][1].BalanceMinor; main != 1_249_475_000 || cb != 0 {
		t.Errorf("balances = main %d, cash back %d; want one redemption", main, cb)
	}
}

func TestRedeemCashbackRefusals(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name, user, key, value string
		wantCode               int
		wantErrCode            string
	}{
		{"more than the balance", andiID, "redeem-0001", "44750.01", http.StatusUnprocessableEntity, "insufficient_funds"},
		{"nothing to redeem", andiID, "redeem-0001", "0", http.StatusBadRequest, "invalid_amount"},
		{"no idempotency key", andiID, "", "100", http.StatusBadRequest, "invalid_idempotency_key"},
		{"not signed in", "", "redeem-0001", "100", http.StatusUnauthorized, ""},
		{"no cash back account", sarahID, "redeem-0001", "100", http.StatusUnprocessableEntity, "unsupported_currency"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := withCashback(4_475_000)
			w := redeem(testRouter(s), tt.user, tt.key, tt.value)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.wantCode, w.Body)
			}
			if code := errorCode(t, w); code != tt.wantErrCode {
				t.Errorf("code = %q, want %q", code, tt.wantErrCode)
			}
			if cb := s.accounts[andiID][1].BalanceMinor; cb != 4_475_000 {
				t.Errorf("cash back balance = %d, want unchanged", cb)
			}
		})
	}

	t.Run("key reused for a different amount", func(t *testing.T) {
		router := testRouter(withCashback(4_475_000))
		redeem(router, andiID, "redeem-0001", "1000")
		w := redeem(router, andiID, "redeem-0001", "2000")
		if w.Code != http.StatusUnprocessableEntity || errorCode(t, w) != "idempotency_key_reused" {
			t.Errorf("got %d %s, want 422 idempotency_key_reused", w.Code, w.Body)
		}
	})
}
