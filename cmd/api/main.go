// Command api is the entrypoint for the SaaS backend monolith.
//
// It loads configuration and hands off to run, which sets up observability,
// connects to Postgres, applies migrations, wires every module into the HTTP
// server, and starts the background workers (outbox relay and job runner).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/observability"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("load config", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("api exited with error", "error", err)
		os.Exit(1)
	}
}

// newLogger builds the JSON logger with the standard correlation fields
// (request ID, tenant ID, trace ID) attached from context.
func newLogger(level string) *slog.Logger {
	return observability.NewLogger(os.Stdout, level, observability.DefaultFields()...)
}
