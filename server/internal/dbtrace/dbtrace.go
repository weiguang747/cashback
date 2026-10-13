// Package dbtrace measures database time per request and logs slow queries,
// so the access log shows where a request's time went: in queries
// (including waiting on row locks, such as a campaign under load) or
// waiting for a connection.
//
// Queries are identified by their sqlc name ("GetUser"), never by SQL text
// or arguments, which can hold user data.
package dbtrace

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cashback/server/internal/logx"
)

// Stats are one request's database totals. Safe for concurrent use.
type Stats struct {
	queries atomic.Int64
	query   atomic.Int64 // nanoseconds running queries, lock waits included
	wait    atomic.Int64 // nanoseconds waiting for a connection or a slot
}

func (s *Stats) Queries() int64           { return s.queries.Load() }
func (s *Stats) QueryTime() time.Duration { return time.Duration(s.query.Load()) }
func (s *Stats) WaitTime() time.Duration  { return time.Duration(s.wait.Load()) }
func (s *Stats) addQuery(d time.Duration) { s.queries.Add(1); s.query.Add(int64(d)) }
func (s *Stats) addWait(d time.Duration)  { s.wait.Add(int64(d)) }

type statsKey struct{}

// WithStats returns ctx carrying a new Stats for the request.
func WithStats(ctx context.Context) (context.Context, *Stats) {
	s := &Stats{}
	return context.WithValue(ctx, statsKey{}, s), s
}

func from(ctx context.Context) *Stats {
	s, _ := ctx.Value(statsKey{}).(*Stats)
	return s
}

// AddWait records time the request spent waiting to use the database
// outside the pool, e.g. for a money-operation slot.
func AddWait(ctx context.Context, d time.Duration) {
	if s := from(ctx); s != nil {
		s.addWait(d)
	}
}

// Tracer is a pgx query tracer and pgxpool acquire tracer. Set it as
// ConnConfig.Tracer; the pool picks up the acquire half itself.
type Tracer struct {
	// Slow is the duration from which a query is logged at warn.
	Slow time.Duration
}

var (
	_ pgx.QueryTracer       = Tracer{}
	_ pgxpool.AcquireTracer = Tracer{}
)

type queryKey struct{}

type queryStart struct {
	name string
	at   time.Time
}

func (Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryKey{}, queryStart{name: Name(data.SQL), at: time.Now()})
}

func (t Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryKey{}).(queryStart)
	if !ok {
		return
	}
	d := time.Since(start.at)
	if s := from(ctx); s != nil {
		s.addQuery(d)
	}

	level := slog.LevelDebug
	if t.Slow > 0 && d >= t.Slow {
		level = slog.LevelWarn
	}
	log := logx.From(ctx)
	if !log.Enabled(ctx, level) {
		return
	}
	msg := "query"
	if level == slog.LevelWarn {
		msg = "slow query"
	}
	log.LogAttrs(ctx, level, msg,
		slog.String("query", start.name),
		slog.Float64("duration_ms", ms(d)),
		slog.Bool("failed", data.Err != nil),
	)
}

type acquireKey struct{}

func (Tracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, acquireKey{}, time.Now())
}

func (Tracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if at, ok := ctx.Value(acquireKey{}).(time.Time); ok {
		AddWait(ctx, time.Since(at))
	}
}

// Name is a query's name for logs: the sqlc name from the "-- name: X"
// comment sqlc puts first, else the statement's first word ("begin",
// "commit"). Never the rest of the text.
func Name(sql string) string {
	sql = strings.TrimSpace(sql)
	if rest, ok := strings.CutPrefix(sql, "-- name: "); ok {
		if name, _, ok := strings.Cut(rest, " "); ok {
			return name
		}
	}
	word, _, _ := strings.Cut(sql, " ")
	word, _, _ = strings.Cut(word, "\n")
	return strings.ToLower(strings.TrimRight(word, ";"))
}

// MS is a duration in milliseconds, to two decimals, for log attributes.
func MS(d time.Duration) float64 { return ms(d) }

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()/10) / 100
}
