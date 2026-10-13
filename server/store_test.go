package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jackc/pgx/v5"

	"cashback/server/internal/db"
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
