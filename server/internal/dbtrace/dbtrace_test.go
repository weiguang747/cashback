package dbtrace

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cashback/server/internal/logx"
)

func TestName(t *testing.T) {
	for sql, want := range map[string]string{
		"-- name: GetUser :one\nSELECT id, email FROM users WHERE id = $1": "GetUser",
		"-- name: RecordAward :one\nWITH debit AS (...)":                   "RecordAward",
		"begin":            "begin",
		"commit":           "commit",
		"  SELECT 1;":      "select",
		"select\n* from x": "select",
	} {
		if got := Name(sql); got != want {
			t.Errorf("Name(%q) = %q, want %q", sql, got, want)
		}
	}
}

// trace runs one query through the tracer as pgx would, taking d.
func trace(ctx context.Context, tr Tracer, sql string, d time.Duration, err error) {
	ctx = tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: sql, Args: []any{"andi@example.com"}})
	time.Sleep(d)
	tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: err})
}

func TestStatsAndSlowQueryLog(t *testing.T) {
	var buf bytes.Buffer
	ctx := logx.WithLogger(context.Background(), logx.New(&buf, slog.LevelInfo).With("request_id", "req-1"))
	ctx, stats := WithStats(ctx)
	tr := Tracer{Slow: 20 * time.Millisecond}

	trace(ctx, tr, "-- name: GetUser :one\nSELECT id FROM users WHERE email = $1", 0, nil)
	trace(ctx, tr, "-- name: LockActiveCampaign :one\nSELECT ... FOR UPDATE", 25*time.Millisecond, errors.New("boom"))
	AddWait(ctx, 3*time.Millisecond)

	if stats.Queries() != 2 || stats.QueryTime() < 25*time.Millisecond || stats.WaitTime() != 3*time.Millisecond {
		t.Errorf("stats = %d queries, %v query, %v wait", stats.Queries(), stats.QueryTime(), stats.WaitTime())
	}

	logs := buf.String()
	// Only the slow one, at warn, by name, with the request's id.
	if strings.Count(logs, "\n") != 1 || !strings.Contains(logs, `"msg":"slow query"`) ||
		!strings.Contains(logs, `"query":"LockActiveCampaign"`) || !strings.Contains(logs, `"request_id":"req-1"`) ||
		!strings.Contains(logs, `"failed":true`) {
		t.Errorf("logs = %s, want one slow query line for LockActiveCampaign", logs)
	}
	// Never the SQL text or the arguments.
	for _, leak := range []string{"SELECT", "FOR UPDATE", "andi", "boom"} {
		if strings.Contains(logs, leak) {
			t.Errorf("logs contain %q: %s", leak, logs)
		}
	}
}

func TestDebugLogsEveryQuery(t *testing.T) {
	var buf bytes.Buffer
	ctx := logx.WithLogger(context.Background(), logx.New(&buf, slog.LevelDebug))
	trace(ctx, Tracer{Slow: time.Hour}, "-- name: GetUser :one\nSELECT 1", 0, nil)
	if !strings.Contains(buf.String(), `"msg":"query"`) || !strings.Contains(buf.String(), `"query":"GetUser"`) {
		t.Errorf("debug logs = %s, want a query line", buf.String())
	}
}

func TestOutsideARequest(t *testing.T) {
	// No Stats in the context (startup, migrations, CLI tools): nothing to add to, no panic.
	trace(context.Background(), Tracer{}, "begin", 0, nil)
	AddWait(context.Background(), time.Second)
}
