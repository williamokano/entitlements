// Package testkit provides shared integration-test infrastructure.
//
// Postgres(t) hands each test a fully migrated, isolated database backed by a
// single postgres:16 testcontainer that is started once per test binary.
// Integration tests (files guarded by `//go:build integration`) must use this
// package instead of hand-rolling containers or mocking SQL.
//
// The container image can be overridden with TESTKIT_POSTGRES_IMAGE (useful
// behind registry proxies, e.g. mirror.gcr.io/library/postgres:16-alpine).
package testkit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/williamokano/entitlements/internal/platform/postgres"
)

const (
	defaultImage  = "postgres:16-alpine"
	adminUser     = "testkit"
	adminPassword = "testkit"
	adminDB       = "postgres"
)

var (
	once      sync.Once
	adminDSN  string // DSN of the admin database on the shared container
	startErr  error
	dbCounter atomic.Int64
)

// PostgresDSN returns the DSN of a freshly created, fully migrated database
// that is exclusive to the calling test. The underlying container is shared
// per test binary; each call creates a new database on it, so parallel tests
// never interfere.
func PostgresDSN(t testing.TB) string {
	t.Helper()

	once.Do(startContainer)
	if startErr != nil {
		t.Fatalf("testkit: start postgres container: %v (is Docker running?)", startErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create an isolated database for this test on the shared container.
	name := fmt.Sprintf("testkit_%d_%d", os.Getpid(), dbCounter.Add(1))
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("testkit: connect admin db: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s OWNER %s", name, adminUser)); err != nil {
		t.Fatalf("testkit: create database %s: %v", name, err)
	}

	dsn := replaceDatabase(adminDSN, name)
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatalf("testkit: migrate %s: %v", name, err)
	}
	return dsn
}

// Postgres returns a pgx pool connected to a freshly created, fully migrated
// database exclusive to the calling test. The pool is closed automatically
// when the test finishes.
func Postgres(t testing.TB) *pgxpool.Pool {
	t.Helper()

	dsn := PostgresDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("testkit: connect %s: %v", dsn, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// startContainer boots the shared postgres container. It intentionally does
// not register termination: the testcontainers reaper (ryuk) removes it after
// the test binary exits; where ryuk is unavailable, set
// TESTCONTAINERS_RYUK_DISABLED=true and the ephemeral environment absorbs it.
func startContainer() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	image := os.Getenv("TESTKIT_POSTGRES_IMAGE")
	if image == "" {
		image = defaultImage
	}

	container, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase(adminDB),
		tcpostgres.WithUsername(adminUser),
		tcpostgres.WithPassword(adminPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		startErr = err
		return
	}

	adminDSN, startErr = container.ConnectionString(ctx, "sslmode=disable")
}

// replaceDatabase swaps the database name (path segment) of a keyword=value
// free URL DSN like postgres://user:pass@host:port/db?params.
func replaceDatabase(dsn, db string) string {
	// The container DSN always has the shape .../<adminDB>?<params>.
	const marker = "/" + adminDB + "?"
	for i := len(dsn) - len(marker); i >= 0; i-- {
		if dsn[i:i+len(marker)] == marker {
			return dsn[:i+1] + db + dsn[i+len(marker)-1:]
		}
	}
	return dsn
}
