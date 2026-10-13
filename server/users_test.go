package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/db"
)

func TestRequireUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := testRouter(demoStore(1))

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
	router := testRouter(fakeStore{err: errors.New("db down")})

	w := get(router, "/v1/accounts", andiID)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (a database error is not a sign-in problem)", w.Code)
	}
}

// countingStore counts GetUser calls.
type countingStore struct {
	fakeStore
	getUser *int
}

func (s countingStore) GetUser(ctx context.Context, id string) (db.GetUserRow, error) {
	*s.getUser++
	return s.fakeStore.GetUser(ctx, id)
}

func TestRequireUserCachesUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	router := testRouter(countingStore{fakeStore: demoStore(1), getUser: &calls})

	for range 3 {
		if w := get(router, "/v1/accounts", andiID); w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body)
		}
	}
	if calls != 1 {
		t.Errorf("GetUser called %d times for one user, want 1", calls)
	}

	// Unknown users aren't cached: each attempt is checked and refused.
	unknown := "33333333-3333-4333-8333-333333333333"
	for range 2 {
		if w := get(router, "/v1/accounts", unknown); w.Code != http.StatusUnauthorized {
			t.Errorf("unknown user: status = %d, want 401", w.Code)
		}
	}
	if calls != 3 {
		t.Errorf("GetUser called %d times, want 3 (unknown users looked up every time)", calls)
	}
}

func TestUserCache(t *testing.T) {
	now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	uc := newUserCache(30*time.Second, 2)
	uc.now = func() time.Time { return now }

	andi := user{ID: andiID, Email: "andi@example.com", Name: "Andi"}
	uc.put(andi)
	if got, ok := uc.get(andiID); !ok || got != andi {
		t.Fatalf("get = %+v, %v; want andi", got, ok)
	}
	now = now.Add(30 * time.Second)
	if _, ok := uc.get(andiID); ok {
		t.Error("entry still served after its TTL")
	}

	// Full: expired entries go first, then everything if none had expired.
	uc.put(user{ID: sarahID})
	uc.put(user{ID: "a"})
	if len(uc.entries) != 2 {
		t.Errorf("entries = %d, want the expired one dropped to make room", len(uc.entries))
	}
	uc.put(user{ID: "b"})
	if len(uc.entries) > 2 {
		t.Errorf("entries = %d, want at most 2", len(uc.entries))
	}
}

func TestListAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := testRouter(demoStore(0))

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
