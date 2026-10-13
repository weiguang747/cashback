// Package database connects to Postgres and brings its schema up to date.
package database

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"cashback/server/internal/dbtrace"
	"cashback/server/migrations"
)

// DefaultMaxConns is the pool size unless DATABASE_URL sets pool_max_conns.
// pgx's own default is the CPU count, which is 2-4 in a small container:
// too few once payments queue on a campaign lock. Keep it well under the
// server's max_connections (100 by default) times the number of instances.
const DefaultMaxConns = 20

// SlowQuery is the duration from which a query is logged at warn, by name,
// with the request's id (see internal/dbtrace).
const SlowQuery = 100 * time.Millisecond

// Open connects to the database at rawURL and applies any pending migrations.
func Open(ctx context.Context, rawURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if u, err := url.Parse(rawURL); err != nil || !u.Query().Has("pool_max_conns") {
		cfg.MaxConns = DefaultMaxConns
	}
	// Per-request database time for the access log, and slow-query logs.
	cfg.ConnConfig.Tracer = dbtrace.Tracer{Slow: SlowQuery}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Migrate applies the embedded migrations that haven't run yet. Goose
// records applied versions in goose_db_version, so this is safe to call on
// every start.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
