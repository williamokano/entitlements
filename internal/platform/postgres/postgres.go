// Package postgres provides the pgx connection pool, the UnitOfWork
// transaction manager (transactions travel in context so repositories join
// the ambient transaction), and the goose-based migration runner.
//
// Each module owns its own Postgres schema; no cross-module joins.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// minPoolConns is a floor on the pool size. The job runner elects a leader per
// job with a Postgres advisory lock held on a dedicated connection for the whole
// run, so every job that is due concurrently holds one connection while its work
// needs another; with several modules registering jobs (renewal, trial, grace,
// dunning, entitlements) the pgx default of max(4, numCPU) starves the runner on
// low-core hosts and deadlocks. This floor keeps enough headroom for the runner,
// the outbox relay, and request handlers to make progress together. A deployment
// that needs more can still raise it via the DSN's pool_max_conns.
const minPoolConns = 16

// NewPool creates a pgx connection pool from a DSN and verifies connectivity.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	if cfg.MaxConns < minPoolConns {
		cfg.MaxConns = minPoolConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}
