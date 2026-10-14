package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/db"
)

func launchCampaign() *db.GetCampaignStatusRow {
	return &db.GetCampaignStatusRow{
		ID: "camp-1", Name: "Launch 5%", Currency: "IDR", MinorUnit: 2, RateBps: 500,
		MinSpendMinor: 2_000_000, DailyCapMinor: 5_000_000, BudgetRemainingMinor: 999_825_000,
		Active: true, EarnedTodayMinor: 175_000, ResetAt: time.Date(2026, 10, 13, 17, 0, 0, 0, time.UTC),
	}
}

func TestPaymentReportsCashback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("campaign live", func(t *testing.T) {
		s := demoStore(0)
		s.campaign = launchCampaign()
		router := testRouter(s)

		first := pay(router, andiID, "key-00000001", kopi)
		for _, want := range []string{
			`"cashback":{"amount":{"value":"1750.00","currency":"IDR"},"reason":"awarded","status":"credited"}`,
			// The payment row itself carries what it earned.
			`"kind":"out","createdAt":"2026-10-13T08:00:00Z","cashback":{"value":"1750.00","currency":"IDR"}`,
		} {
			if !strings.Contains(first.Body.String(), want) {
				t.Errorf("body missing %s\nbody: %s", want, first.Body)
			}
		}
		if replay := pay(router, andiID, "key-00000001", kopi); replay.Body.String() != first.Body.String() {
			t.Errorf("replay = %s, want the same cash back as %s", replay.Body, first.Body)
		}
	})

	t.Run("awarded through the gate", func(t *testing.T) {
		s := demoStore(0)
		s.campaign, s.gate = launchCampaign(), "redis"
		w := pay(testRouter(s), andiID, "key-00000001", kopi)
		want := `"cashback":{"amount":{"value":"1750.00","currency":"IDR"},"reason":"awarded","status":"pending"}`
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body missing %s\nbody: %s", want, w.Body)
		}
	})

	t.Run("gate unavailable", func(t *testing.T) {
		s := demoStore(0)
		s.campaign, s.gate = launchCampaign(), "down"
		w := pay(testRouter(s), andiID, "key-00000001", kopi)
		if w.Code != 201 {
			t.Fatalf("status = %d, want the payment to go ahead: %s", w.Code, w.Body)
		}
		want := `"cashback":{"amount":{"value":"0.00","currency":"IDR"},"reason":"unavailable","status":"credited"}`
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body missing %s\nbody: %s", want, w.Body)
		}
		if strings.Count(w.Body.String(), `"cashback"`) != 1 {
			t.Errorf("transaction shouldn't carry cashback when none was earned: %s", w.Body)
		}
	})

	t.Run("no campaign", func(t *testing.T) {
		w := pay(testRouter(demoStore(0)), andiID, "key-00000001", kopi)
		if !strings.Contains(w.Body.String(), `"cashback":null`) {
			t.Errorf("body = %s, want \"cashback\":null", w.Body)
		}
		if strings.Contains(w.Body.String(), `"transaction":{`) && strings.Count(w.Body.String(), `"cashback"`) != 1 {
			t.Errorf("transaction shouldn't carry cashback when none was earned: %s", w.Body)
		}
	})
}

func TestGetCampaign(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if body := get(testRouter(demoStore(0)), "/v1/cashback/campaign", andiID).Body.String(); body != `{"campaign":null}` {
		t.Errorf("no campaign: body = %s", body)
	}

	s := demoStore(0)
	s.campaign = launchCampaign()
	want := `{"campaign":{"id":"camp-1","name":"Launch 5%","status":"active","rateBps":500,` +
		`"minSpend":{"value":"20000.00","currency":"IDR"},"dailyCap":{"value":"50000.00","currency":"IDR"},` +
		`"earnedToday":{"value":"1750.00","currency":"IDR"},"budgetRemaining":{"value":"9998250.00","currency":"IDR"},` +
		`"resetAt":"2026-10-13T17:00:00Z"}}`
	if body := get(testRouter(s), "/v1/cashback/campaign", andiID).Body.String(); body != want {
		t.Errorf("body = %s\nwant   %s", body, want)
	}

	s.campaign.Active = false
	if body := get(testRouter(s), "/v1/cashback/campaign", andiID).Body.String(); !strings.Contains(body, `"status":"ended"`) {
		t.Errorf("ended campaign: body = %s", body)
	}

	if w := get(testRouter(s), "/v1/cashback/campaign", ""); w.Code != 401 {
		t.Errorf("signed out: status = %d, want 401", w.Code)
	}
}

func TestTransactionsShowEarnedCashback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(2)
	earned := int64(175_000)
	s.txns[andiID][0].CashbackMinor = &earned

	body := get(testRouter(s), "/v1/transactions", andiID).Body.String()
	if strings.Count(body, `"cashback":{"value":"1750.00","currency":"IDR"}`) != 1 || strings.Count(body, `"cashback"`) != 1 {
		t.Errorf("want cashback on exactly the first payment, body: %s", body)
	}
}

func TestCashbackHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(0)
	s.accounts[andiID][1].BalanceMinor = 175_000
	s.cashbackTxns = map[string][]db.ListCashbackTransactionsRow{
		andiID: {{ID: "cb1", Name: "Cash back", Description: "From Kopi Senja", Kind: "cashback",
			AmountMinor: 175_000, Currency: "IDR", MinorUnit: 2, CreatedAt: time.Date(2026, 10, 13, 2, 42, 0, 0, time.UTC)}},
	}
	router := testRouter(s)

	want := `{"balance":{"value":"1750.00","currency":"IDR"},"transactions":[{"id":"cb1","name":"Cash back",` +
		`"description":"From Kopi Senja","amount":{"value":"1750.00","currency":"IDR"},"kind":"cashback","createdAt":"2026-10-13T02:42:00Z"}],"nextCursor":null}`
	if body := get(router, "/v1/cashback/history", andiID).Body.String(); body != want {
		t.Errorf("body = %s\nwant   %s", body, want)
	}

	// Sarah has no accounts in the demo store, and never sees Andi's rows.
	if w := get(router, "/v1/cashback/history", sarahID); w.Code != 404 || strings.Contains(w.Body.String(), "cb1") {
		t.Errorf("sarah: %d %s, want 404 without Andi's rows", w.Code, w.Body)
	}
	if w := get(router, "/v1/cashback/history?limit=0", andiID); w.Code != 400 {
		t.Errorf("limit=0: status = %d, want 400", w.Code)
	}
}

// noAccountLookup fails GetUserAccount, to show a request didn't need it.
type noAccountLookup struct{ fakeStore }

func (noAccountLookup) GetUserAccount(context.Context, db.GetUserAccountParams) (db.GetUserAccountRow, error) {
	return db.GetUserAccountRow{}, errors.New("account looked up")
}

func TestCashbackHistoryBalanceOnFirstPageOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := demoStore(0)
	s.accounts[andiID][1].BalanceMinor = 350_000
	for i := range 3 {
		s.cashbackTxns = map[string][]db.ListCashbackTransactionsRow{andiID: append(s.cashbackTxns[andiID], db.ListCashbackTransactionsRow{
			ID: fmt.Sprintf("00000000-0000-4000-8000-00000000000%d", i), Name: "Cash back", Description: "From Kopi Senja",
			Kind: "cashback", AmountMinor: 175_000, Currency: "IDR", MinorUnit: 2,
			CreatedAt: time.Date(2026, 10, 13, 2, 42-i, 0, 0, time.UTC),
		})}
	}

	var first struct {
		Balance    *struct{ Value string }
		NextCursor *string
	}
	w := get(testRouter(s), "/v1/cashback/history?limit=2", andiID)
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil || first.Balance == nil || first.Balance.Value != "3500.00" || first.NextCursor == nil {
		t.Fatalf("first page = %d %s, want the balance and a cursor", w.Code, w.Body)
	}

	// The next page doesn't look the account up and says so with null.
	w = get(testRouter(noAccountLookup{s}), "/v1/cashback/history?limit=2&cursor="+*first.NextCursor, andiID)
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), `{"balance":null,"transactions":[{"id":"00000000-0000-4000-8000-000000000002"`) {
		t.Errorf("second page = %d %s, want balance null and the third row", w.Code, w.Body)
	}
}
