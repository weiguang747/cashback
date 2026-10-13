package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
	"cashback/server/internal/idempotency"
)

const (
	andiID  = "11111111-1111-4111-8111-111111111111"
	sarahID = "22222222-2222-4222-8222-222222222222"
)

var wib = time.FixedZone("WIB", 7*60*60)

// fakeStore serves fixed rows the way the real queries do: per user,
// newest first, and pgx.ErrNoRows for unknown users.
type fakeStore struct {
	users    []db.GetUserRow
	accounts map[string][]db.ListAccountsRow     // by user id
	txns     map[string][]db.ListTransactionsRow // by user id
	err      error
	payErr   error                                           // returned by Pay when set
	paid     map[string]db.GetTransactionByIdempotencyKeyRow // by idempotency key
}

func (f fakeStore) GetUser(_ context.Context, id string) (db.GetUserRow, error) {
	if f.err != nil {
		return db.GetUserRow{}, f.err
	}
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return db.GetUserRow{}, pgx.ErrNoRows
}

func (f fakeStore) GetUserByEmail(_ context.Context, email string) (db.GetUserByEmailRow, error) {
	if f.err != nil {
		return db.GetUserByEmailRow{}, f.err
	}
	for _, u := range f.users {
		if u.Email == email {
			return db.GetUserByEmailRow(u), nil
		}
	}
	return db.GetUserByEmailRow{}, pgx.ErrNoRows
}

func (f fakeStore) ListAccounts(_ context.Context, userID string) ([]db.ListAccountsRow, error) {
	return f.accounts[userID], f.err
}

func (f fakeStore) ListTransactions(_ context.Context, arg db.ListTransactionsParams) ([]db.ListTransactionsRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	rows := f.txns[arg.UserID]
	if arg.Limit != nil && int(*arg.Limit) < len(rows) {
		return rows[:*arg.Limit], nil
	}
	return rows, nil
}

func (f fakeStore) GetMainAccount(_ context.Context, arg db.GetMainAccountParams) (db.GetMainAccountRow, error) {
	if f.err != nil {
		return db.GetMainAccountRow{}, f.err
	}
	for _, a := range f.accounts[arg.UserID] {
		if a.Kind == "main" && a.Currency == arg.Currency {
			return db.GetMainAccountRow{ID: a.ID, Currency: a.Currency, MinorUnit: a.MinorUnit, BalanceMinor: a.BalanceMinor}, nil
		}
	}
	return db.GetMainAccountRow{}, pgx.ErrNoRows
}

// Pay mimics database.Pay: refuses to overdraw, debits in place, and
// replays a key that already paid.
func (f fakeStore) Pay(_ context.Context, p database.PayParams) (database.PayResult, error) {
	if f.payErr != nil {
		return database.PayResult{}, f.payErr
	}
	for _, accounts := range f.accounts {
		for i := range accounts {
			a := &accounts[i]
			if a.ID != p.AccountID {
				continue
			}
			if row, ok := f.paid[p.IdempotencyKey]; ok {
				if row.AmountMinor != -p.AmountMinor {
					return database.PayResult{}, database.ErrKeyReused
				}
				return database.PayResult{Transaction: row, BalanceMinor: a.BalanceMinor, Replayed: true}, nil
			}
			if a.BalanceMinor < p.AmountMinor {
				return database.PayResult{}, database.ErrInsufficientFunds
			}
			a.BalanceMinor -= p.AmountMinor
			row := db.GetTransactionByIdempotencyKeyRow{
				ID: "pay-" + p.IdempotencyKey, Name: p.MerchantName, Description: "Payment", Kind: "out",
				AmountMinor: -p.AmountMinor, Currency: p.Currency, MinorUnit: a.MinorUnit,
				CreatedAt: time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC),
			}
			f.paid[p.IdempotencyKey] = row
			return database.PayResult{Transaction: row, BalanceMinor: a.BalanceMinor}, nil
		}
	}
	return database.PayResult{}, pgx.ErrNoRows
}

// fakeIdem is an in-memory idempotency.Store.
type fakeIdem struct {
	mu      sync.Mutex
	records map[string]fakeIdemRecord
	err     error // returned by Begin when set
}

type fakeIdemRecord struct {
	fingerprint string
	done        bool
	resp        idempotency.Response
}

func newFakeIdem() *fakeIdem { return &fakeIdem{records: map[string]fakeIdemRecord{}} }

func (f *fakeIdem) Begin(_ context.Context, scope, key, fp string) (idempotency.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return idempotency.Result{}, f.err
	}
	rec, ok := f.records[scope+":"+key]
	switch {
	case !ok:
		f.records[scope+":"+key] = fakeIdemRecord{fingerprint: fp}
		return idempotency.Result{Outcome: idempotency.Started}, nil
	case rec.fingerprint != fp:
		return idempotency.Result{Outcome: idempotency.Mismatch}, nil
	case rec.done:
		return idempotency.Result{Outcome: idempotency.Replay, Response: rec.resp}, nil
	default:
		return idempotency.Result{Outcome: idempotency.InProgress}, nil
	}
}

func (f *fakeIdem) Complete(_ context.Context, scope, key, fp string, resp idempotency.Response) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[scope+":"+key] = fakeIdemRecord{fingerprint: fp, done: true, resp: resp}
	return nil
}

func (f *fakeIdem) Release(_ context.Context, scope, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.records, scope+":"+key)
	return nil
}

func testRouter(s store) *gin.Engine { return setupRouter(s, newFakeIdem()) }

// demoStore has Andi with n transactions and Sarah with one.
func demoStore(n int) fakeStore {
	andi := make([]db.ListTransactionsRow, n)
	for i := range andi {
		andi[i] = db.ListTransactionsRow{
			ID: "t" + string(rune('a'+i)), Name: "Kopi Kenangan", Description: "Payment", Kind: "out",
			AmountMinor: -3_500_000, Currency: "IDR", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 13, 8, 42-i, 0, 0, wib),
		}
	}
	return fakeStore{
		paid: map[string]db.GetTransactionByIdempotencyKeyRow{},
		users: []db.GetUserRow{
			{ID: andiID, Email: "andi@example.com", Name: "Andi Pratama"},
			{ID: sarahID, Email: "sarah@example.com", Name: "Sarah Wijaya"},
		},
		accounts: map[string][]db.ListAccountsRow{
			andiID: {
				{ID: "acc-main", Kind: "main", Currency: "IDR", MinorUnit: 2, BalanceMinor: 1_245_000_000},
				{ID: "acc-cb", Kind: "cashback", Currency: "IDR", MinorUnit: 2, BalanceMinor: 0},
			},
		},
		txns: map[string][]db.ListTransactionsRow{
			andiID: andi,
			sarahID: {{ID: "s1", Name: "Netflix", Description: "Subscription", Kind: "out",
				AmountMinor: -18_600_000, Currency: "IDR", MinorUnit: 2, CreatedAt: time.Date(2026, 10, 9, 23, 0, 0, 0, wib)}},
		},
	}
}

// get sends a GET as the given user ("" for no X-User-ID header).
func get(h http.Handler, path, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
