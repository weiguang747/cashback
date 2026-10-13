package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
)

const kopi = `{"merchantId":"kopi-senja","merchantName":"Kopi Senja","amount":{"value":"35000","currency":"IDR"}}`

// pay sends POST /v1/payments as Andi ("" key or user means no header).
func pay(h http.Handler, userID, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(body))
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

func mainBalance(s fakeStore) int64 { return s.accounts[andiID][0].BalanceMinor }

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp httpx.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Error == "" {
		t.Fatalf("body = %s, want a JSON error", w.Body)
	}
	return resp.Code
}

func TestCreatePayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(0)
	router := testRouter(s)

	w := pay(router, andiID, "key-00000001", kopi)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	for _, want := range []string{
		`"name":"Kopi Senja"`,
		`"amount":{"value":"-35000.00","currency":"IDR"}`,
		`"kind":"out"`,
		`"balance":{"value":"12415000.00","currency":"IDR"}`,
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body missing %s\nbody: %s", want, w.Body)
		}
	}
	if got := mainBalance(s); got != 1_241_500_000 {
		t.Errorf("balance = %d, want 1241500000", got)
	}
}

func TestCreatePaymentIdempotency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(0)
	router := testRouter(s)

	first := pay(router, andiID, "key-00000001", kopi)
	second := pay(router, andiID, "key-00000001", kopi)
	if second.Code != first.Code || second.Body.String() != first.Body.String() {
		t.Errorf("replay = %d %s, want %d %s", second.Code, second.Body, first.Code, first.Body)
	}
	if second.Header().Get("Idempotent-Replayed") != "true" {
		t.Error("replay is missing Idempotent-Replayed header")
	}
	if got := mainBalance(s); got != 1_241_500_000 {
		t.Errorf("balance = %d after a replay, want one debit (1241500000)", got)
	}

	// Same key, different payment.
	other := strings.Replace(kopi, `"35000"`, `"50000"`, 1)
	w := pay(router, andiID, "key-00000001", other)
	if w.Code != http.StatusUnprocessableEntity || errorCode(t, w) != "idempotency_key_reused" {
		t.Errorf("reused key = %d %s, want 422 idempotency_key_reused", w.Code, w.Body)
	}

	// A new key is a new payment.
	if w := pay(router, andiID, "key-00000002", kopi); w.Code != http.StatusCreated {
		t.Errorf("new key = %d, want 201", w.Code)
	}
	if got := mainBalance(s); got != 1_238_000_000 {
		t.Errorf("balance = %d, want two debits (1238000000)", got)
	}
}

func TestCreatePaymentInProgress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	idem := newFakeIdem()
	router := setupRouter(demoStore(0), idem)
	// Another request with this key started and hasn't finished.
	if _, err := idem.Begin(t.Context(), andiID, "key-00000001", fingerprint("payment", "kopi-senja", "Kopi Senja", "35000", "IDR")); err != nil {
		t.Fatal(err)
	}
	w := pay(router, andiID, "key-00000001", kopi)
	if w.Code != http.StatusConflict || errorCode(t, w) != "in_progress" {
		t.Errorf("got %d %s, want 409 in_progress", w.Code, w.Body)
	}
}

func TestCreatePaymentInsufficientFunds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(0)
	router := testRouter(s)

	tooMuch := strings.Replace(kopi, `"35000"`, `"12450000.01"`, 1)
	w := pay(router, andiID, "key-00000001", tooMuch)
	if w.Code != http.StatusUnprocessableEntity || errorCode(t, w) != "insufficient_funds" {
		t.Fatalf("got %d %s, want 422 insufficient_funds", w.Code, w.Body)
	}
	if got := mainBalance(s); got != 1_245_000_000 {
		t.Errorf("balance = %d, want unchanged", got)
	}

	// The whole balance is fine.
	exact := strings.Replace(kopi, `"35000"`, `"12450000"`, 1)
	if w := pay(router, andiID, "key-00000002", exact); w.Code != http.StatusCreated {
		t.Errorf("paying the exact balance = %d %s, want 201", w.Code, w.Body)
	}
	if got := mainBalance(s); got != 0 {
		t.Errorf("balance = %d, want 0", got)
	}
}

func TestCreatePaymentValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := testRouter(demoStore(0))
	amount := func(v, cur string) string {
		return `{"merchantId":"kopi-senja","merchantName":"Kopi Senja","amount":{"value":"` + v + `","currency":"` + cur + `"}}`
	}

	tests := []struct {
		name, user, key, body string
		wantCode              int
		wantErrCode           string
	}{
		{"not signed in", "", "key-00000001", kopi, http.StatusUnauthorized, ""},
		{"no idempotency key", andiID, "", kopi, http.StatusBadRequest, "invalid_idempotency_key"},
		{"short idempotency key", andiID, "abc", kopi, http.StatusBadRequest, "invalid_idempotency_key"},
		{"bad characters in key", andiID, "key with spaces", kopi, http.StatusBadRequest, "invalid_idempotency_key"},
		{"malformed JSON", andiID, "key-00000001", `{"merchantId":`, http.StatusBadRequest, ""},
		{"no merchant", andiID, "key-00000001", `{"amount":{"value":"1","currency":"IDR"}}`, http.StatusBadRequest, "invalid_merchant"},
		{"blank merchant name", andiID, "key-00000001", `{"merchantId":"x","merchantName":"   ","amount":{"value":"1","currency":"IDR"}}`, http.StatusBadRequest, "invalid_merchant"},
		{"merchant name too long", andiID, "key-00000001", `{"merchantId":"x","merchantName":"` + strings.Repeat("é", maxMerchantLen+1) + `","amount":{"value":"1","currency":"IDR"}}`, http.StatusBadRequest, "invalid_merchant"},
		{"merchant name at the limit", andiID, "key-00000001", `{"merchantId":"x","merchantName":"` + strings.Repeat("é", maxMerchantLen) + `","amount":{"value":"1","currency":"IDR"}}`, http.StatusCreated, ""},
		{"zero", andiID, "key-00000001", amount("0", "IDR"), http.StatusBadRequest, "invalid_amount"},
		{"negative", andiID, "key-00000001", amount("-5", "IDR"), http.StatusBadRequest, "invalid_amount"},
		{"too many decimals", andiID, "key-00000001", amount("1.234", "IDR"), http.StatusBadRequest, "invalid_amount"},
		{"not a number", andiID, "key-00000001", amount("1e3", "IDR"), http.StatusBadRequest, "invalid_amount"},
		{"no account in currency", andiID, "key-00000001", amount("10", "USD"), http.StatusUnprocessableEntity, "unsupported_currency"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := pay(router, tt.user, tt.key, tt.body)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.wantCode, w.Body)
			}
			if w.Code < 400 {
				return
			}
			if code := errorCode(t, w); code != tt.wantErrCode {
				t.Errorf("code = %q, want %q", code, tt.wantErrCode)
			}
		})
	}
}

func TestCreatePaymentFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("redis down fails closed", func(t *testing.T) {
		s := demoStore(0)
		idem := newFakeIdem()
		idem.err = errors.New("connection refused")
		w := pay(setupRouter(s, idem), andiID, "key-00000001", kopi)
		if w.Code != http.StatusServiceUnavailable || errorCode(t, w) != "unavailable" {
			t.Errorf("got %d %s, want 503 unavailable", w.Code, w.Body)
		}
		if got := mainBalance(s); got != 1_245_000_000 {
			t.Errorf("balance = %d, want unchanged", got)
		}
	})

	t.Run("database error releases the key for a retry", func(t *testing.T) {
		s := demoStore(0)
		s.payErr = errors.New("db down")
		idem := newFakeIdem()
		router := setupRouter(s, idem)
		if w := pay(router, andiID, "key-00000001", kopi); w.Code != http.StatusInternalServerError {
			t.Fatalf("got %d, want 500", w.Code)
		}

		s.payErr = nil
		router = setupRouter(s, idem)
		if w := pay(router, andiID, "key-00000001", kopi); w.Code != http.StatusCreated {
			t.Errorf("retry with the same key = %d %s, want 201", w.Code, w.Body)
		}
	})

	t.Run("database catches a key Redis lost", func(t *testing.T) {
		s := demoStore(0)
		pay(testRouter(s), andiID, "key-00000001", kopi)
		// A fresh fakeIdem has forgotten the key, like a Redis flush.
		w := pay(testRouter(s), andiID, "key-00000001", kopi)
		if w.Code != http.StatusCreated || w.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("got %d replayed=%q, want 201 replayed", w.Code, w.Header().Get("Idempotent-Replayed"))
		}
		if got := mainBalance(s); got != 1_241_500_000 {
			t.Errorf("balance = %d, want one debit", got)
		}
	})
}
