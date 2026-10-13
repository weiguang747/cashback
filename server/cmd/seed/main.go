// Command seed fills an empty database with demo transactions placed
// relative to "today", so the demo always shows recent activity.
//
//	DATABASE_URL=postgres://... go run ./cmd/seed
//
// Demo rows live here, not in a migration, so they never reach production.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
)

// wib is Western Indonesia Time (UTC+7), where the demo user lives.
var wib = time.FixedZone("WIB", 7*60*60)

type demoTransaction struct {
	name, description     string
	amountMinor           int64 // sen: rupiah × 100
	currency, kind        string
	daysAgo, hour, minute int
}

var demoTransactions = []demoTransaction{
	{"Kopi Kenangan", "Payment", -3_500_000, "IDR", "out", 0, 8, 42},
	{"Sarah Wijaya", "Received", 50_000_000, "IDR", "in", 0, 7, 30},
	{"Indomaret", "Payment", -24_850_000, "IDR", "out", 1, 19, 15},
	{"Gojek Ride", "Payment", -4_200_000, "IDR", "out", 1, 18, 2},
	{"Cash back redeemed", "To available balance", 6_400_000, "IDR", "cashback", 1, 9, 10},
	{"Salary · PT Acme", "Received", 950_000_000, "IDR", "in", 3, 9, 0},
	{"PLN Electricity", "Bill", -35_000_000, "IDR", "out", 3, 8, 15},
	{"Netflix", "Subscription", -18_600_000, "IDR", "out", 4, 23, 0},
	{"Tokopedia", "Payment", -12_900_000, "IDR", "out", 4, 14, 40},
	{"Budi Santoso", "Received", 7_500_000, "IDR", "in", 4, 12, 5},
	{"Shell Fuel", "Payment", -20_000_000, "IDR", "out", 5, 18, 20},
	{"Kopi Kenangan", "Payment", -3_500_000, "IDR", "out", 5, 8, 35},
}

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions").Scan(&count); err != nil {
		log.Fatal(err)
	}
	if count > 0 {
		log.Printf("transactions already has %d rows; nothing to seed", count)
		return
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)

	q := db.New(tx)
	today := time.Now().In(wib)
	for _, d := range demoTransactions {
		day := today.AddDate(0, 0, -d.daysAgo)
		if _, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
			Name:        d.name,
			Description: d.description,
			Kind:        d.kind,
			AmountMinor: d.amountMinor,
			Currency:    d.currency,
			CreatedAt:   time.Date(day.Year(), day.Month(), day.Day(), d.hour, d.minute, 0, 0, wib),
		}); err != nil {
			log.Fatalf("insert %s: %v", d.name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded %d transactions", len(demoTransactions))
}
