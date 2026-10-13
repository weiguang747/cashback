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
		{"limit over the maximum", andiID, "?limit=101", http.StatusBadRequest, 0},
		{"limit over int32", andiID, "?limit=9999999999", http.StatusBadRequest, 0},
		{"kind filter", andiID, "?kind=in", http.StatusOK, 0},
		{"unknown kind", andiID, "?kind=refund", http.StatusBadRequest, 0},
		{"search", andiID, "?q=KOPI", http.StatusOK, 12},
		{"search with no match", andiID, "?q=netflix", http.StatusOK, 0},
		{"search too long", andiID, "?q=" + strings.Repeat("a", 101), http.StatusBadRequest, 0},
		{"garbage cursor", andiID, "?cursor=not-a-cursor", http.StatusBadRequest, 0},
		{"cursor with a bad id", andiID, "?cursor=" + encodeCursor(time.Now(), "x'; drop table"), http.StatusBadRequest, 0},
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

// Following nextCursor visits every row exactly once, newest first, and the
// last page says there is no next one.
func TestListTransactionsPaging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(12)
	// Two rows at the same moment, either side of a page break: the id
	// breaks the tie (descending, like the query).
	rows := s.txns[andiID]
	rows[5].CreatedAt = rows[4].CreatedAt
	rows[4].ID, rows[5].ID = rows[5].ID, rows[4].ID
	router := testRouter(s)

	var seen []string
	pages := 0
	path := "/v1/transactions?limit=5"
	for {
		w := get(router, path, andiID)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d (body: %s)", w.Code, w.Body)
		}
		var resp struct {
			Transactions []struct{ ID string } `json:"transactions"`
			NextCursor   *string               `json:"nextCursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		pages++
		for _, tx := range resp.Transactions {
			seen = append(seen, tx.ID)
		}
		if resp.NextCursor == nil {
			break
		}
		path = "/v1/transactions?limit=5&cursor=" + *resp.NextCursor
	}

	if pages != 3 {
		t.Errorf("pages = %d, want 3 (5 + 5 + 2)", pages)
	}
	if len(seen) != 12 {
		t.Fatalf("saw %d rows, want 12: %v", len(seen), seen)
	}
	for i, id := range seen {
		if id != s.txns[andiID][i].ID {
			t.Errorf("row %d = %s, want %s", i, id, s.txns[andiID][i].ID)
		}
	}
}

// Search text is matched literally: LIKE wildcards in it are escaped.
func TestListTransactionsSearchEscapesWildcards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got db.ListTransactionsParams
	s := capturingTransactions{fakeStore: demoStore(0), got: &got}
	if w := get(testRouter(s), `/v1/transactions?q=+50%25_off\+&kind=out`, andiID); w.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", w.Code, w.Body)
	}
	if got.NamePattern == nil || *got.NamePattern != `%50\%\_off\\%` {
		t.Errorf("pattern = %v, want %%50\\%%\\_off\\\\%%", got.NamePattern)
	}
	if got.Kind == nil || *got.Kind != "out" {
		t.Errorf("kind = %v, want out", got.Kind)
	}
	if got.Limit != defaultPageSize+1 {
		t.Errorf("limit = %d, want %d (page size + 1)", got.Limit, defaultPageSize+1)
	}
}

type capturingTransactions struct {
	fakeStore
	got *db.ListTransactionsParams
}

func (c capturingTransactions) ListTransactions(ctx context.Context, arg db.ListTransactionsParams) ([]db.ListTransactionsRow, error) {
	*c.got = arg
	return c.fakeStore.ListTransactions(ctx, arg)
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
	router := testRouter(failingTransactions{s, errors.New("db down")})

	w := get(router, "/v1/transactions", andiID)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "db down") {
		t.Errorf("status = %d body = %s, want 500 without internal details", w.Code, w.Body)
	}
}

// failingTransactions finds users but fails to list their transactions.
type failingTransactions struct {
	fakeStore
	err error
}

func (f failingTransactions) ListTransactions(_ context.Context, _ db.ListTransactionsParams) ([]db.ListTransactionsRow, error) {
	return nil, f.err
}

func TestSlots(t *testing.T) {
	s := newSlots(2)
	ctx := context.Background()
	if err := s.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.acquire(ctx); err != nil {
		t.Fatal(err)
	}

	// Full: the wait ends with the request's deadline, as a deadline error,
	// so the handler answers 503 "timeout".
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := s.acquire(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("acquire when full = %v, want DeadlineExceeded", err)
	}

	s.release()
	if err := s.acquire(ctx); err != nil {
		t.Errorf("acquire after release = %v", err)
	}
}

func TestMoneySlotsLeaveHalfThePool(t *testing.T) {
	for _, tt := range []struct {
		maxConns int32
		want     int
	}{{20, 10}, {4, 2}, {3, 1}, {1, 1}} {
		if got := moneySlots(tt.maxConns); got != tt.want {
			t.Errorf("moneySlots(%d) = %d, want %d", tt.maxConns, got, tt.want)
		}
	}
}
