package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/cashback"
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
	// Cash back: the campaign the user sees, their cash back account rows,
	// and what Pay reports earning (5% when a campaign is set).
	campaign     *db.GetCampaignStatusRow
	cashbackTxns map[string][]db.ListCashbackTransactionsRow // by user id
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
	var out []db.ListTransactionsRow
	for _, r := range f.txns[arg.UserID] {
		if !beforeCursor(r.CreatedAt, r.ID, arg.BeforeCreatedAt, arg.BeforeID) ||
			(arg.Kind != nil && r.Kind != *arg.Kind) ||
			(arg.NamePattern != nil && !likeMatch(r.Name, *arg.NamePattern)) {
			continue
		}
		out = append(out, r)
	}
	return limitRows(out, arg.Limit), nil
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

func (f fakeStore) GetUserAccount(_ context.Context, arg db.GetUserAccountParams) (db.GetUserAccountRow, error) {
	if f.err != nil {
		return db.GetUserAccountRow{}, f.err
	}
	for _, a := range f.accounts[arg.UserID] {
		if a.Kind == arg.Kind && a.Currency == arg.Currency {
			return db.GetUserAccountRow{ID: a.ID, BalanceMinor: a.BalanceMinor, MinorUnit: a.MinorUnit}, nil
		}
	}
	return db.GetUserAccountRow{}, pgx.ErrNoRows
}

func (f fakeStore) GetCampaignStatus(_ context.Context, arg db.GetCampaignStatusParams) (db.GetCampaignStatusRow, error) {
	if f.err != nil {
		return db.GetCampaignStatusRow{}, f.err
	}
	if f.campaign == nil || f.campaign.Currency != arg.Currency {
		return db.GetCampaignStatusRow{}, pgx.ErrNoRows
	}
	return *f.campaign, nil
}

func (f fakeStore) ListCashbackTransactions(_ context.Context, arg db.ListCashbackTransactionsParams) ([]db.ListCashbackTransactionsRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []db.ListCashbackTransactionsRow
	for _, r := range f.cashbackTxns[arg.UserID] {
		if beforeCursor(r.CreatedAt, r.ID, arg.BeforeCreatedAt, arg.BeforeID) {
			out = append(out, r)
		}
	}
	return limitRows(out, arg.Limit), nil
}

// beforeCursor is the queries' (created_at, id) < (cursor) row comparison;
// rows are already newest first. A nil cursor is the first page.
func beforeCursor(at time.Time, id string, cursorAt *time.Time, cursorID *string) bool {
	if cursorAt == nil {
		return true
	}
	return at.Before(*cursorAt) || (at.Equal(*cursorAt) && id < *cursorID)
}

// likeMatch is ILIKE for the "%text%" patterns listTransactions builds.
func likeMatch(name, pattern string) bool {
	text := strings.NewReplacer(`\%`, "%", `\_`, "_", `\\`, `\`).Replace(strings.TrimSuffix(strings.TrimPrefix(pattern, "%"), "%"))
	return strings.Contains(strings.ToLower(name), strings.ToLower(text))
}

func limitRows[T any](rows []T, limit int32) []T {
	if int(limit) < len(rows) {
		return rows[:limit]
	}
	return rows
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
			var cb *database.CashbackResult
			if f.campaign != nil {
				cb = &database.CashbackResult{CampaignID: f.campaign.ID, AmountMinor: p.AmountMinor / 20, Reason: cashback.Awarded}
			}
			return database.PayResult{Transaction: row, BalanceMinor: a.BalanceMinor, Cashback: cb}, nil
		}
	}
	return database.PayResult{}, pgx.ErrNoRows
}

// Redeem mimics database.Redeem: moves cash back to main, refuses to
// redeem more than the balance, and replays a key that already redeemed.
func (f fakeStore) Redeem(_ context.Context, p database.RedeemParams) (database.RedeemResult, error) {
	if f.payErr != nil {
		return database.RedeemResult{}, f.payErr
	}
	accounts := f.accounts[p.UserID]
	var main, cb *db.ListAccountsRow
	for i := range accounts {
		switch accounts[i].Kind {
		case "main":
			main = &accounts[i]
		case "cashback":
			cb = &accounts[i]
		}
	}
	if main == nil || cb == nil {
		return database.RedeemResult{}, database.ErrNoCashbackAccount
	}
	if row, ok := f.paid[p.IdempotencyKey]; ok {
		if row.AmountMinor != p.AmountMinor {
			return database.RedeemResult{}, database.ErrKeyReused
		}
		return database.RedeemResult{Transaction: row, MainBalanceMinor: main.BalanceMinor,
			CashbackBalanceMinor: cb.BalanceMinor, MinorUnit: cb.MinorUnit, Replayed: true}, nil
	}
	if cb.BalanceMinor < p.AmountMinor {
		return database.RedeemResult{}, database.ErrInsufficientFunds
	}
	cb.BalanceMinor -= p.AmountMinor
	main.BalanceMinor += p.AmountMinor
	row := db.GetTransactionByIdempotencyKeyRow{
		ID: "redeem-" + p.IdempotencyKey, Name: "Cash back redeemed", Description: "To available balance",
		Kind: "cashback", AmountMinor: p.AmountMinor, Currency: p.Currency, MinorUnit: cb.MinorUnit,
		CreatedAt: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC),
	}
	f.paid[p.IdempotencyKey] = row
	return database.RedeemResult{Transaction: row, MainBalanceMinor: main.BalanceMinor,
		CashbackBalanceMinor: cb.BalanceMinor, MinorUnit: cb.MinorUnit}, nil
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
			ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", i), Name: "Kopi Kenangan", Description: "Payment", Kind: "out",
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
			sarahID: {{ID: "00000000-0000-4000-8000-100000000001", Name: "Netflix", Description: "Subscription", Kind: "out",
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
