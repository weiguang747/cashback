// Command campaign launches, lists and ends cash back campaigns.
//
//	DATABASE_URL=postgres://... go run ./cmd/campaign create --name "Launch 5%" \
//	    --currency IDR --rate-bps 500 --min 20000 --daily-cap 50000 \
//	    --budget 10000000 --round-to 1 --starts-at now [--gate redis]
//	DATABASE_URL=postgres://... go run ./cmd/campaign list
//	DATABASE_URL=postgres://... go run ./cmd/campaign end <campaign-id>
//	DATABASE_URL=postgres://... REDIS_URL=redis://... go run ./cmd/campaign gate <campaign-id> redis
//	DATABASE_URL=postgres://... REDIS_URL=redis://... go run ./cmd/campaign reconcile <campaign-id>
//
// Launching moves the budget from the currency's funding account into a new
// campaign account; ending returns whatever is left. Amounts are in major
// units (rupiah), parsed exactly.
//
// New campaigns use the Redis budget gate unless --gate says otherwise, so
// create needs REDIS_URL too. The campaign is launched with the db gate and
// then switched (as gate does), because only the switch seeds the counter.
//
// gate switches how the budget is enforced (db, shadow or redis; see
// database.Reconciler.SetGate). reconcile runs one reconciler pass and
// corrects the Redis counter. Both need REDIS_URL (default
// redis://localhost:6379/0), and so does ending a campaign that uses Redis.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cashback/server/internal/budgetgate"
	"cashback/server/internal/database"
	"cashback/server/internal/db"
	"cashback/server/internal/money"
)

const usage = `usage:
  campaign create --name NAME --budget AMOUNT [flags]   launch a campaign
  campaign list                                          show all campaigns
  campaign end CAMPAIGN_ID                               end now and return the unspent budget
  campaign gate CAMPAIGN_ID db|shadow|redis              switch how the budget is enforced
  campaign reconcile CAMPAIGN_ID                         check the Redis counter and correct drift`

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal(usage)
	}
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

	switch os.Args[1] {
	case "create":
		err = create(ctx, pool, os.Args[2:], func() *database.Reconciler { return reconciler(ctx, pool, true) })
	case "list":
		err = list(ctx, pool)
	case "end":
		if len(os.Args) != 3 {
			log.Fatal(usage)
		}
		err = end(ctx, pool, reconciler(ctx, pool, false), os.Args[2])
	case "gate":
		if len(os.Args) != 4 {
			log.Fatal(usage)
		}
		err = reconciler(ctx, pool, true).SetGate(ctx, os.Args[2], os.Args[3])
		if err == nil {
			fmt.Printf("%s now uses the %s budget gate\n", os.Args[2], os.Args[3])
		}
	case "reconcile":
		if len(os.Args) != 3 {
			log.Fatal(usage)
		}
		err = reconcile(ctx, reconciler(ctx, pool, true), os.Args[2])
	default:
		log.Fatal(usage)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func create(ctx context.Context, pool *pgxpool.Pool, args []string, reconciler func() *database.Reconciler) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	name := fs.String("name", "", "campaign name shown to users (required)")
	currency := fs.String("currency", "IDR", "ISO 4217 currency")
	rateBps := fs.Int("rate-bps", 500, "cash back rate in basis points (500 = 5%)")
	minSpend := fs.String("min", "0", "minimum payment that earns cash back")
	dailyCap := fs.String("daily-cap", "", "most cash back one user can earn per day (required)")
	budget := fs.String("budget", "", "total budget; the campaign ends when it's used up (required)")
	roundTo := fs.String("round-to", "1", "awards are rounded down to a multiple of this")
	timezone := fs.String("timezone", "Asia/Jakarta", "time zone that defines the daily cap's day")
	startsAt := fs.String("starts-at", "now", `"now" or an RFC 3339 time`)
	endsAt := fs.String("ends-at", "", "optional RFC 3339 end time")
	gate := fs.String("gate", database.GateRedis, "budget gate: db, shadow or redis")
	fs.Parse(args)
	if *name == "" || *dailyCap == "" || *budget == "" {
		fs.Usage()
		return errors.New("--name, --daily-cap and --budget are required")
	}
	if *gate != database.GateDB && *gate != database.GateShadow && *gate != database.GateRedis {
		return fmt.Errorf("--gate: %w", database.ErrBadGate)
	}
	// Connect to Redis before launching, so an unreachable Redis stops here
	// rather than leaving a campaign on the wrong gate.
	var r *database.Reconciler
	if *gate != database.GateDB {
		r = reconciler()
	}

	var minorUnit int
	if err := pool.QueryRow(ctx, "SELECT minor_unit FROM currencies WHERE code = $1", *currency).Scan(&minorUnit); err != nil {
		return fmt.Errorf("unknown currency %q", *currency)
	}
	parse := func(flagName, v string, allowZero bool) (int64, error) {
		if allowZero && (v == "0" || v == "") {
			return 0, nil
		}
		n, err := money.Parse(v, minorUnit)
		if err != nil {
			return 0, fmt.Errorf("--%s: %w", flagName, err)
		}
		return n, nil
	}
	minMinor, err := parse("min", *minSpend, true)
	if err != nil {
		return err
	}
	capMinor, err := parse("daily-cap", *dailyCap, false)
	if err != nil {
		return err
	}
	budgetMinor, err := parse("budget", *budget, false)
	if err != nil {
		return err
	}
	incrementMinor, err := parse("round-to", *roundTo, false)
	if err != nil {
		return err
	}

	start := time.Now()
	if *startsAt != "now" {
		if start, err = time.Parse(time.RFC3339, *startsAt); err != nil {
			return fmt.Errorf("--starts-at: %w", err)
		}
	}
	var endPtr *time.Time
	if *endsAt != "" {
		e, err := time.Parse(time.RFC3339, *endsAt)
		if err != nil {
			return fmt.Errorf("--ends-at: %w", err)
		}
		endPtr = &e
	}

	id, err := database.LaunchCampaign(ctx, pool, database.LaunchParams{
		Name: *name, Currency: *currency, RateBps: int32(*rateBps),
		MinSpendMinor: minMinor, DailyCapMinor: capMinor, AwardIncrementMinor: incrementMinor,
		BudgetMinor: budgetMinor, Timezone: *timezone, StartsAt: start, EndsAt: endPtr,
	})
	if errors.Is(err, database.ErrCampaignOverlap) {
		return fmt.Errorf("another %s campaign is already scheduled for that time; end it first (campaign list)", *currency)
	}
	if err != nil {
		return err
	}
	if r != nil {
		if err := r.SetGate(ctx, id, *gate); err != nil {
			return fmt.Errorf("launched %s (%s) with the db gate, but switching to %s failed: %w; retry with: campaign gate %s %s",
				*name, id, *gate, err, id, *gate)
		}
	}
	fmt.Printf("launched %s (%s) with the %s budget gate\n", *name, id, *gate)
	return nil
}

// reconciler connects to Redis. Without it (required false and Redis
// unreachable), only campaigns that don't use the gate can be ended.
func reconciler(ctx context.Context, pool *pgxpool.Pool, required bool) *database.Reconciler {
	r := &database.Reconciler{Pool: pool}
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379/0"
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		if required {
			log.Fatalf("redis unavailable: %v", err)
		}
		return r
	}
	r.Gate = budgetgate.NewRedis(client)
	return r
}

func reconcile(ctx context.Context, r *database.Reconciler, id string) error {
	res, err := r.Reconcile(ctx, id, true)
	if err != nil {
		return err
	}
	if !res.Open {
		fmt.Printf("%s: counter closed (gate db, or being switched); pruned %d, released %d, %d in flight\n",
			id, res.Pruned, res.Released, res.InFlight)
		return nil
	}
	fmt.Printf("%s: drift %d minor units (corrected: %v); pruned %d, released %d, %d in flight\n",
		id, res.Drift, res.Corrected, res.Pruned, res.Released, res.InFlight)
	return nil
}

func end(ctx context.Context, pool *pgxpool.Pool, r *database.Reconciler, id string) error {
	returned, err := database.EndCampaign(ctx, pool, r, id)
	if err != nil {
		return err
	}
	var currency string
	var minorUnit int
	if err := pool.QueryRow(ctx, `SELECT c.currency, cur.minor_unit FROM campaigns c
		JOIN currencies cur ON cur.code = c.currency WHERE c.id = $1`, id).Scan(&currency, &minorUnit); err != nil {
		return err
	}
	left := money.Money{Minor: returned, Currency: currency, Exponent: minorUnit}
	fmt.Printf("ended %s; returned %s %s to funding\n", id, left.Decimal(), currency)
	return nil
}

func list(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := db.New(pool).ListCampaigns(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tGATE\tRATE\tMIN\tDAILY CAP\tREMAINING / BUDGET\tSTARTS\tENDS")
	now := time.Now()
	for _, c := range rows {
		m := func(minor int64) string {
			return money.Money{Minor: minor, Currency: c.Currency, Exponent: int(c.MinorUnit)}.Decimal()
		}
		status := "active"
		switch {
		case c.StartsAt.After(now):
			status = "scheduled"
		case c.EndsAt != nil && !c.EndsAt.After(now):
			status = "ended"
		}
		gate := c.BudgetGate
		if c.GateDegraded {
			gate += " (degraded)"
		}
		ends := "-"
		if c.EndsAt != nil {
			ends = c.EndsAt.In(time.Local).Format(time.DateTime)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.2f%%\t%s\t%s\t%s / %s %s\t%s\t%s\n",
			c.ID, c.Name, status, gate, float64(c.RateBps)/100, m(c.MinSpendMinor), m(c.DailyCapMinor),
			m(c.RemainingMinor), m(c.BudgetMinor), c.Currency, c.StartsAt.In(time.Local).Format(time.DateTime), ends)
	}
	return w.Flush()
}
