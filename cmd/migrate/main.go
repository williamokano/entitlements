// Command migrate applies or rolls back database migrations using the
// embedded per-module migration files.
//
// Usage:
//
//	migrate up             # apply all pending migrations for every module
//	migrate down <module>  # roll back the most recent migration of one module
//
// The database is taken from DATABASE_URL (see internal/platform/config).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()

	switch {
	case len(args) == 1 && args[0] == "up":
		if err := postgres.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
		fmt.Println("migrations applied")
		return nil
	case len(args) == 2 && args[0] == "down":
		if err := postgres.MigrateDown(ctx, cfg.DatabaseURL, args[1]); err != nil {
			return err
		}
		fmt.Printf("rolled back one migration of module %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("usage: migrate up | migrate down <module>")
	}
}
