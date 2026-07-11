// Command api is the entrypoint for the SaaS backend monolith.
//
// It loads configuration and starts an HTTP server (via platform/httpx) with
// the standard middleware chain and liveness/readiness probes. The real
// composition root — wiring Postgres, the event bus/outbox, jobs, and every
// module's router — arrives in T-009.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/httpx"
	"github.com/williamokano/entitlements/internal/platform/id"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("load config", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)

	srv := httpx.NewServer(":"+cfg.HTTPPort, newHandler(logger), logger)

	// Serve until an interrupt/terminate signal arrives, then shut down
	// gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}

// newHandler builds the top-level HTTP handler: the standard middleware chain
// plus health probes. In T-009 this grows into the composition root that
// mounts every module's router.
func newHandler(logger *slog.Logger) http.Handler {
	router := httpx.NewRouter(httpx.DefaultMiddleware(logger, id.UUIDv7{})...)
	router.HandleFunc("GET /healthz", httpx.Health())
	router.HandleFunc("GET /readyz", httpx.Health())
	return router.Handler()
}

// newLogger builds a JSON slog logger at the given level (defaulting to info
// for unrecognized values).
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
