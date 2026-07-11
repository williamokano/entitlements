//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

// probe gives each test a fresh database, a UnitOfWork, and a scratch table.
func probe(t *testing.T) (*pgxpool.Pool, *postgres.UnitOfWork, context.Context) {
	t.Helper()
	pool := testkit.Postgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE platform.uow_probe (v text)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	return pool, postgres.NewUnitOfWork(pool), ctx
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform.uow_probe`).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestUnitOfWorkCommits(t *testing.T) {
	pool, uow, ctx := probe(t)

	err := uow.Do(ctx, func(ctx context.Context) error {
		_, err := postgres.Q(ctx, pool).Exec(ctx, `INSERT INTO platform.uow_probe VALUES ('committed')`)
		return err
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := countRows(t, ctx, pool); got != 1 {
		t.Fatalf("rows after commit = %d, want 1", got)
	}
}

func TestUnitOfWorkRollsBackOnError(t *testing.T) {
	pool, uow, ctx := probe(t)

	sentinel := errors.New("boom")
	err := uow.Do(ctx, func(ctx context.Context) error {
		if _, err := postgres.Q(ctx, pool).Exec(ctx, `INSERT INTO platform.uow_probe VALUES ('doomed')`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do error = %v, want sentinel", err)
	}
	if got := countRows(t, ctx, pool); got != 0 {
		t.Fatalf("rows after rollback = %d, want 0", got)
	}
}

func TestUnitOfWorkRollsBackOnPanic(t *testing.T) {
	pool, uow, ctx := probe(t)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic did not propagate out of Do")
			}
		}()
		_ = uow.Do(ctx, func(ctx context.Context) error {
			if _, err := postgres.Q(ctx, pool).Exec(ctx, `INSERT INTO platform.uow_probe VALUES ('doomed')`); err != nil {
				return err
			}
			panic("boom")
		})
	}()

	if got := countRows(t, ctx, pool); got != 0 {
		t.Fatalf("rows after panic = %d, want 0", got)
	}
}

func TestNestedDoJoinsOuterTransaction(t *testing.T) {
	pool, uow, ctx := probe(t)

	sentinel := errors.New("outer failure")
	err := uow.Do(ctx, func(ctx context.Context) error {
		// Inner Do joins the same transaction and succeeds on its own...
		if err := uow.Do(ctx, func(ctx context.Context) error {
			_, err := postgres.Q(ctx, pool).Exec(ctx, `INSERT INTO platform.uow_probe VALUES ('inner')`)
			return err
		}); err != nil {
			return err
		}
		if !postgres.InTx(ctx) {
			t.Error("outer fn does not see ambient tx")
		}
		// ...but the outer failure must roll the inner write back too.
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do error = %v, want sentinel", err)
	}
	if got := countRows(t, ctx, pool); got != 0 {
		t.Fatalf("inner write survived outer rollback: rows = %d, want 0", got)
	}
}

func TestQueryOutsideDoRunsNonTransactional(t *testing.T) {
	pool, _, ctx := probe(t)

	if postgres.InTx(ctx) {
		t.Fatal("fresh context reports an ambient tx")
	}
	if _, err := postgres.Q(ctx, pool).Exec(ctx, `INSERT INTO platform.uow_probe VALUES ('direct')`); err != nil {
		t.Fatalf("direct exec via Q: %v", err)
	}
	if got := countRows(t, ctx, pool); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}
}

func TestMigrateDownReverts(t *testing.T) {
	dsn := testkit.PostgresDSN(t)
	ctx := context.Background()

	if err := postgres.MigrateDown(ctx, dsn, "platform"); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	schemaExists := func() bool {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'billing')`).Scan(&exists)
		if err != nil {
			t.Fatalf("check schema: %v", err)
		}
		return exists
	}

	if schemaExists() {
		t.Fatal("billing schema still present after down migration")
	}

	// And MigrateUp restores it (round trip).
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatalf("MigrateUp after down: %v", err)
	}
	if !schemaExists() {
		t.Fatal("billing schema missing after re-applying migrations")
	}
}
