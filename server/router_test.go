package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
)

// Every failure, including ones the router produces itself, must use the
// {"error": "..."} shape the app relies on.
func TestErrorsAreJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(demoStore(1))

	tests := []struct {
		name, method, path, body string
		wantCode                 int
	}{
		{"handler error", http.MethodPost, "/v1/auth/sign-in", `{"email":"nope"}`, http.StatusBadRequest},
		{"bad query param", http.MethodGet, "/v1/transactions?limit=x", "", http.StatusBadRequest},
		{"unknown route", http.MethodGet, "/v1/nope", "", http.StatusNotFound},
		{"unversioned path", http.MethodGet, "/transactions", "", http.StatusNotFound},
		{"wrong method", http.MethodDelete, "/v1/transactions", "", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-ID", andiID)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.wantCode, w.Body)
			}
			var resp httpx.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Error == "" {
				t.Errorf("body = %s, want {\"error\": \"...\"}", w.Body)
			}
		})
	}
}

func TestPanicReturnsJSONError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(fakeStore{})
	router.GET("/panic", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Errorf("body = %s, want JSON error", w.Body)
	}
}
