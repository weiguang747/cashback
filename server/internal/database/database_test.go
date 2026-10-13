package database

import (
	"context"
	"os"
	"testing"
	"time"

	"cashback/server/internal/db"
)

// Runs against a real Postgres only when TEST_DATABASE_URL is set, e.g.
//
//	TEST_DATABASE_URL=postgres://cashback:cashback@localhost:5432/cashback_test?sslmode=disable go test ./...
//
// Use a throwaway database: the test deletes the rows it inserts.
func TestStoreRoundTrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Migrations are idempotent.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	// Above JS's Number.MAX_SAFE_INTEGER (2^53 - 1): must survive exactly.
	const big = int64(9_007_199_254_740_993)
	at := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	if _, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
		Name: "Big", Description: "Received", Kind: "in", AmountMinor: big, Currency: "USD", CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}

	limit := int32(1)
	rows, err := q.ListTransactions(ctx, &limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.AmountMinor != big || r.Currency != "USD" || r.MinorUnit != 2 || !r.CreatedAt.Equal(at) {
		t.Errorf("round trip = %+v", r)
	}
}

func TestConstraints(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tests := []struct {
		name string
		p    db.InsertTransactionParams
	}{
		{"money in with negative amount", db.InsertTransactionParams{Kind: "in", AmountMinor: -100, Currency: "IDR"}},
		{"money out with positive amount", db.InsertTransactionParams{Kind: "out", AmountMinor: 100, Currency: "IDR"}},
		{"zero amount", db.InsertTransactionParams{Kind: "in", AmountMinor: 0, Currency: "IDR"}},
		{"unknown kind", db.InsertTransactionParams{Kind: "refund", AmountMinor: 100, Currency: "IDR"}},
		{"unknown currency", db.InsertTransactionParams{Kind: "in", AmountMinor: 100, Currency: "XXX"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.p.Name, tt.p.Description, tt.p.CreatedAt = "x", "x", time.Now()
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := db.New(tx).InsertTransaction(ctx, tt.p); err == nil {
				t.Error("insert succeeded, want constraint violation")
			}
		})
	}
}
