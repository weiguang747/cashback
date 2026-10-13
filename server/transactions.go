package main

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
)

type transaction struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"` // whole rupiah; negative = money out
	Kind        string    `json:"kind"`   // "in", "out" or "cashback"
	CreatedAt   time.Time `json:"createdAt"`
}

type transactionsResponse struct {
	Transactions []transaction `json:"transactions"`
}

// wib is Western Indonesia Time (UTC+7). A fixed zone avoids needing
// tzdata in the container image.
var wib = time.FixedZone("WIB", 7*60*60)

// demoTransaction is seed data placed relative to "today" so the demo
// always shows recent activity. Replace with a database later.
type demoTransaction struct {
	id, name, description string
	amount                int64
	kind                  string
	daysAgo, hour, minute int
}

var demoTransactions = []demoTransaction{
	{"t1", "Kopi Kenangan", "Payment", -35_000, "out", 0, 8, 42},
	{"t2", "Sarah Wijaya", "Received", 500_000, "in", 0, 7, 30},
	{"t3", "Indomaret", "Payment", -248_500, "out", 1, 19, 15},
	{"t4", "Gojek Ride", "Payment", -42_000, "out", 1, 18, 2},
	{"t5", "Cash back redeemed", "To available balance", 64_000, "cashback", 1, 9, 10},
	{"t6", "Salary · PT Acme", "Received", 9_500_000, "in", 3, 9, 0},
	{"t7", "PLN Electricity", "Bill", -350_000, "out", 3, 8, 15},
	{"t8", "Netflix", "Subscription", -186_000, "out", 4, 23, 0},
	{"t9", "Tokopedia", "Payment", -129_000, "out", 4, 14, 40},
	{"t10", "Budi Santoso", "Received", 75_000, "in", 4, 12, 5},
	{"t11", "Shell Fuel", "Payment", -200_000, "out", 5, 18, 20},
	{"t12", "Kopi Kenangan", "Payment", -35_000, "out", 5, 8, 35},
}

// listTransactions returns transactions newest first.
// Optional query param: limit (positive integer).
func listTransactions(c *gin.Context) {
	limit := 0
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			httpx.Error(c, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}

	txns := buildDemoTransactions(time.Now())
	if limit > 0 && limit < len(txns) {
		txns = txns[:limit]
	}
	c.JSON(http.StatusOK, transactionsResponse{Transactions: txns})
}

func buildDemoTransactions(now time.Time) []transaction {
	today := now.In(wib)
	txns := make([]transaction, 0, len(demoTransactions))
	for _, d := range demoTransactions {
		day := today.AddDate(0, 0, -d.daysAgo)
		txns = append(txns, transaction{
			ID:          d.id,
			Name:        d.name,
			Description: d.description,
			Amount:      d.amount,
			Kind:        d.kind,
			CreatedAt:   time.Date(day.Year(), day.Month(), day.Day(), d.hour, d.minute, 0, 0, wib),
		})
	}
	sort.SliceStable(txns, func(i, j int) bool { return txns[i].CreatedAt.After(txns[j].CreatedAt) })
	return txns
}
