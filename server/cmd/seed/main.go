// Command seed fills the database with demo users, their accounts, and
// transactions placed relative to "today", so the demo always shows recent
// activity. Safe to re-run: users and accounts are upserted, and
// transactions are only inserted into an empty table.
//
//	DATABASE_URL=postgres://... go run ./cmd/seed
//
// Demo rows live here, not in a migration, so they never reach production.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"cashback/server/internal/database"
	"cashback/server/internal/db"
)

// wib is Western Indonesia Time (UTC+7), where the demo users live.
var wib = time.FixedZone("WIB", 7*60*60)

const currency = "IDR"

type demoTransaction struct {
	name, description     string
	amountMinor           int64 // sen: rupiah × 100
	kind                  string
	daysAgo, hour, minute int
}

type demoUser struct {
	email, name string
	// mainBalanceMinor is where the main account ends up. An "Opening
	// balance" row makes up the difference from the transactions below.
	mainBalanceMinor int64
	// Main account history. Payments use merchants from the app's list
	// (app/src/data/merchants.ts). There is no seeded cash back: it is only
	// earned from a live campaign, as a double-entry award (cmd/campaign).
	transactions []demoTransaction
}

var demoUsers = []demoUser{
	{
		email: "andi@example.com", name: "Andi Pratama", mainBalanceMinor: 1_245_000_000,
		transactions: []demoTransaction{
			{"Kopi Senja", "Payment", -3_500_000, "out", 0, 8, 42},
			{"Sarah Wijaya", "Received", 50_000_000, "in", 0, 7, 30},
			{"Toko Maju Jaya", "Payment", -24_850_000, "out", 1, 19, 15},
			{"Martabak Pak Udin", "Payment", -4_200_000, "out", 1, 18, 2},
			{"Salary · PT Acme", "Received", 950_000_000, "in", 3, 9, 0},
		},
	},
	{
		email: "sarah@example.com", name: "Sarah Wijaya", mainBalanceMinor: 320_000_000,
		transactions: []demoTransaction{
			{"Apotek Sehat", "Payment", -8_500_000, "out", 3, 8, 15},
			{"Laundry Bersih", "Payment", -6_000_000, "out", 4, 17, 0},
			{"Pasar Segar", "Payment", -12_900_000, "out", 4, 14, 40},
		},
	},
	{
		email: "budi@example.com", name: "Budi Santoso", mainBalanceMinor: 175_000_000,
		transactions: []demoTransaction{
			{"Andi Pratama", "Received", 7_500_000, "in", 4, 12, 5},
			{"Bengkel Motor Jaya", "Payment", -20_000_000, "out", 5, 18, 20},
			{"Kopi Senja", "Payment", -3_500_000, "out", 5, 8, 35},
		},
	},
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var existing int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM transactions").Scan(&existing); err != nil {
		log.Fatal(err)
	}

	q := db.New(tx)
	inserted := 0
	for _, u := range demoUsers {
		n, err := seedUser(ctx, q, u, existing == 0)
		if err != nil {
			log.Fatalf("seed %s: %v", u.email, err)
		}
		inserted += n
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}

	if existing > 0 {
		log.Printf("upserted %d users; transactions already has %d rows, so none were added", len(demoUsers), existing)
		return
	}
	log.Printf("seeded %d users and %d transactions", len(demoUsers), inserted)
}

// seedUser upserts the user and their accounts, and when withTransactions is
// set, inserts their history and syncs each stored balance to it.
func seedUser(ctx context.Context, q *db.Queries, u demoUser, withTransactions bool) (int, error) {
	userID, err := q.UpsertUser(ctx, db.UpsertUserParams{Email: u.email, Name: u.name})
	if err != nil {
		return 0, err
	}
	accounts := map[string]string{}
	for _, kind := range []string{"main", "cashback"} {
		id, err := q.UpsertAccount(ctx, db.UpsertAccountParams{UserID: userID, Kind: kind, Currency: currency})
		if err != nil {
			return 0, err
		}
		accounts[kind] = id
	}
	if !withTransactions {
		return 0, nil
	}

	var mainSum int64
	for _, t := range u.transactions {
		mainSum += t.amountMinor
	}
	opening := u.mainBalanceMinor - mainSum
	if opening <= 0 {
		return 0, fmt.Errorf("main balance %d is below its transactions' total %d", u.mainBalanceMinor, mainSum)
	}
	rows := append([]demoTransaction{{"Opening balance", "Top up", opening, "in", 30, 9, 0}}, u.transactions...)

	today := time.Now().In(wib)
	for _, t := range rows {
		day := today.AddDate(0, 0, -t.daysAgo)
		if _, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
			AccountID:   accounts["main"],
			Name:        t.name,
			Description: t.description,
			Kind:        t.kind,
			AmountMinor: t.amountMinor,
			Currency:    currency,
			CreatedAt:   time.Date(day.Year(), day.Month(), day.Day(), t.hour, t.minute, 0, 0, wib),
		}); err != nil {
			return 0, fmt.Errorf("insert %s: %w", t.name, err)
		}
	}

	// Store each balance as the sum of its account's transactions.
	for _, id := range accounts {
		if err := q.SyncAccountBalance(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}
