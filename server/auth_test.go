package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSignIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupRouter(demoStore(0))

	tests := []struct {
		name      string
		body      string
		wantCode  int
		wantEmail string
	}{
		{"known email", `{"email":"andi@example.com"}`, http.StatusOK, "andi@example.com"},
		{"normalizes case and spaces", `{"email":"  Andi@Example.COM "}`, http.StatusOK, "andi@example.com"},
		{"unknown email", `{"email":"you@example.com"}`, http.StatusUnauthorized, ""},
		{"empty email", `{"email":""}`, http.StatusBadRequest, ""},
		{"missing email", `{}`, http.StatusBadRequest, ""},
		{"no at sign", `{"email":"not-an-email"}`, http.StatusBadRequest, ""},
		{"no dot in domain", `{"email":"a@b"}`, http.StatusBadRequest, ""},
		{"display name", `{"email":"Name <a@b.co>"}`, http.StatusBadRequest, ""},
		{"malformed JSON", `{"email":`, http.StatusBadRequest, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/sign-in", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.wantCode, w.Body)
			}
			if tt.wantCode != http.StatusOK {
				return
			}

			var resp signInResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if want := (user{ID: andiID, Email: tt.wantEmail, Name: "Andi Pratama"}); resp.User != want {
				t.Errorf("user = %+v, want %+v", resp.User, want)
			}
		})
	}
}
