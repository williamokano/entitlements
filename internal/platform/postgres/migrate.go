package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"

	"github.com/williamokano/entitlements/migrations"
)

// goose keeps package-level state (dialect, base FS, table name), so runs are
// serialized. Migrations are a startup/CLI concern; this is not hot-path.
var gooseMu sync.Mutex

// MigrateUp applies every module's pending migrations, platform first. Each
// module has an independent goose version table (goose_<module>_version).
func MigrateUp(ctx context.Context, dsn string) error {
	mods, err := migrations.Modules()
	if err != nil {
		return fmt.Errorf("postgres: list migration modules: %w", err)
	}
	for _, mod := range mods {
		if err := gooseRun(ctx, dsn, mod, func(db *sql.DB) error {
			return goose.UpContext(ctx, db, ".")
		}); err != nil {
			return fmt.Errorf("postgres: migrate up %s: %w", mod, err)
		}
	}
	return nil
}

// MigrateDown rolls back the most recent migration of one module.
func MigrateDown(ctx context.Context, dsn, module string) error {
	if err := gooseRun(ctx, dsn, module, func(db *sql.DB) error {
		return goose.DownContext(ctx, db, ".")
	}); err != nil {
		return fmt.Errorf("postgres: migrate down %s: %w", module, err)
	}
	return nil
}

// gooseRun opens a database/sql connection, points goose at the module's
// embedded migration directory and per-module version table, and runs fn.
func gooseRun(ctx context.Context, dsn, module string, fn func(db *sql.DB) error) error {
	sub, err := fs.Sub(migrations.FS, module)
	if err != nil {
		return fmt.Errorf("unknown migration module %q: %w", module, err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetBaseFS(sub)
	defer goose.SetBaseFS(nil)
	goose.SetTableName("goose_" + module + "_version")
	defer goose.SetTableName("goose_db_version")
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	return fn(db)
}
