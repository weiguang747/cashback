package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
	"cashback/server/internal/logx"
)

// captureLogs routes the default logger (which setupRouter uses) into a
// buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logx.New(&buf, slog.LevelDebug))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestRequestIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureLogs(t)
	router := testRouter(demoStore(0))

	// A valid incoming id is kept and echoed.
	req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
	req.Header.Set("X-User-ID", andiID)
	req.Header.Set(httpx.RequestIDHeader, "app-req-12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if got := w.Header().Get(httpx.RequestIDHeader); got != "app-req-12345" {
		t.Errorf("echoed request id = %q, want app-req-12345", got)
	}
	if !strings.Contains(logs.String(), `"request_id":"app-req-12345"`) || !strings.Contains(logs.String(), `"user_id":"`+andiID+`"`) {
		t.Errorf("access log missing request_id or user_id:\n%s", logs)
	}

	// A missing or unsafe one is replaced with a generated id.
	for _, incoming := range []string{"", "bad id with spaces", strings.Repeat("x", 100)} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		if incoming != "" {
			req.Header.Set(httpx.RequestIDHeader, incoming)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if got := w.Header().Get(httpx.RequestIDHeader); len(got) != 36 {
			t.Errorf("incoming %q: request id = %q, want a generated UUID", incoming, got)
		}
	}
}

func TestLogsDoNotLeakCredentialsOrEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureLogs(t)
	router := testRouter(demoStore(0))

	signIn := httptest.NewRequest(http.MethodPost, "/v1/auth/sign-in", strings.NewReader(`{"email":"stranger@example.com"}`))
	signIn.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), signIn)

	unknown := "33333333-3333-4333-8333-333333333333"
	router.ServeHTTP(httptest.NewRecorder(), func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/accounts?secret=s3cret", nil)
		r.Header.Set("X-User-ID", unknown)
		r.Header.Set("Authorization", "Bearer tok-123")
		return r
	}())

	out := logs.String()
	for _, leak := range []string{"stranger@example.com", unknown, "tok-123", "s3cret"} {
		if strings.Contains(out, leak) {
			t.Errorf("logs leaked %q:\n%s", leak, out)
		}
	}
	for _, want := range []string{`"msg":"sign in rejected"`, `"email":"s***@e***.com"`, `"reason":"unknown_user"`, `"route":"/v1/accounts"`} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %s:\n%s", want, out)
		}
	}
}

func TestMoneyEventsAreLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureLogs(t)
	s := demoStore(0)
	s.campaign = launchCampaign()
	router := testRouter(s)

	pay(router, andiID, "key-00000001", kopi)
	pay(router, andiID, "key-00000001", kopi) // Redis replay
	pay(router, andiID, "key-00000002", strings.Replace(kopi, `"35000"`, `"99999999"`, 1))

	out := logs.String()
	for _, want := range []string{
		`"msg":"payment completed"`, `"transaction_id":"pay-key-00000001"`, `"cashback_minor":175000`,
		// The completion event itself carries the key, not just the access log.
		`"idempotency_key":"key-00000001","merchant_id":"kopi-senja"`,
		`"msg":"idempotent replay"`, `"source":"redis"`,
		`"msg":"payment declined"`, `"reason":"insufficient_funds"`,
		`"idempotency_key":"key-00000001"`, `"merchant_id":"kopi-senja"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %s:\n%s", want, out)
		}
	}
	// Balances stay out of logs.
	if strings.Contains(out, "balance") {
		t.Errorf("logs mention a balance:\n%s", out)
	}
}

func TestPanicIsLoggedWithRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureLogs(t)
	router := testRouter(demoStore(0))
	router.GET("/boom", func(c *gin.Context) { panic("db password=hunter2 exploded") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set(httpx.RequestIDHeader, "req-panic-1")
	router.ServeHTTP(httptest.NewRecorder(), req)

	out := logs.String()
	if !strings.Contains(out, `"msg":"panic"`) || !strings.Contains(out, `"request_id":"req-panic-1"`) || !strings.Contains(out, `"stack":"goroutine`) {
		t.Errorf("panic log missing message, request id or stack:\n%s", out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("panic log leaked a password:\n%s", out)
	}
}

// A client that disconnects (e.g. the app's timeout) makes in-flight database
// and Redis calls fail with context.Canceled. That isn't a server fault: it
// must not log at error, look like a Redis outage, or count as a 500.
func TestClientCancelIsNotAServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	canceled := func(path, body string) *http.Request {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", andiID)
		req.Header.Set("Idempotency-Key", "key-00000001")
		return req
	}

	tests := []struct {
		name  string
		setup func(*fakeStore, *fakeIdem)
		msg   string
	}{
		{"payment fails mid-flight", func(s *fakeStore, _ *fakeIdem) { s.payErr = context.Canceled },
			"request failed; idempotency key released for retry"},
		{"idempotency lock fails", func(_ *fakeStore, i *fakeIdem) { i.err = context.Canceled },
			"idempotency lock abandoned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)
			s, idem := demoStore(0), newFakeIdem()
			tt.setup(&s, idem)
			w := httptest.NewRecorder()
			setupRouter(s, idem).ServeHTTP(w, canceled("/v1/payments", kopi))

			if w.Code != httpx.StatusClientClosedRequest {
				t.Errorf("status = %d, want %d", w.Code, httpx.StatusClientClosedRequest)
			}
			out := logs.String()
			if strings.Contains(out, `"level":"ERROR"`) || strings.Contains(out, "unavailable") {
				t.Errorf("client cancel logged as a server error:\n%s", out)
			}
			for _, want := range []string{`"msg":"` + tt.msg + `"`, `"client_canceled":true`, `"status":499`} {
				if !strings.Contains(out, want) {
					t.Errorf("logs missing %s:\n%s", want, out)
				}
			}
			if _, locked := idem.records[andiID+":key-00000001"]; locked {
				t.Error("idempotency key still locked; a retry would get in_progress")
			}
		})
	}

	t.Run("real failure is still an error", func(t *testing.T) {
		logs := captureLogs(t)
		s := demoStore(0)
		s.payErr = errors.New("connection reset")
		w := pay(testRouter(s), andiID, "key-00000001", kopi)
		if w.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), `"level":"ERROR"`) {
			t.Errorf("got %d, want 500 logged at error:\n%s", w.Code, logs)
		}
	})
}
