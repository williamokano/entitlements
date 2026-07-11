// Command api is the entrypoint for the SaaS backend monolith.
//
// It loads configuration, sets up observability (structured logging + OpenTelemetry),
// and starts an HTTP server (via platform/httpx) with the standard middleware
// chain and liveness/readiness probes. The real composition root — wiring
// Postgres, the event bus/outbox, jobs, and every module's router — arrives in
// T-009.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/httpx"
	"github.com/williamokano/entitlements/internal/platform/id"
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

	shutdownOTel, err := observability.SetupOTel(ctx, observability.Options{
		ServiceName:  cfg.ServiceName,
		OTLPEndpoint: cfg.OTLPEndpoint,
	})
	if err != nil {
		logger.Error("setup opentelemetry", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownOTel(shutdownCtx); err != nil {
			logger.Error("shutdown opentelemetry", "error", err)
		}
	}()

	srv := httpx.NewServer(":"+cfg.HTTPPort, newHandler(logger), logger)

	if err := srv.Run(ctx); err != nil {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}

// newHandler builds the top-level HTTP handler: the standard middleware chain
// (request ID, recovery, logging) plus a tracing span per request and health
// probes. In T-009 this grows into the composition root that mounts every
// module's router.
func newHandler(logger *slog.Logger) http.Handler {
	// Ordering matters: request ID first, then the tracing span, so both the
	// request ID and the trace ID are already in context when the logging
	// middleware emits its per-request line.
	mws := []httpx.Middleware{
		httpx.RequestID(id.UUIDv7{}),
		observability.TracingMiddleware(otel.GetTracerProvider()),
		httpx.Recovery(logger),
		httpx.Logging(logger),
	}

	router := httpx.NewRouter(mws...)
	router.HandleFunc("GET /healthz", httpx.Health())
	router.HandleFunc("GET /readyz", httpx.Health())
	return router.Handler()
}

// newLogger builds the JSON logger with the standard correlation fields
// (request ID, tenant ID, trace ID) attached from context.
func newLogger(level string) *slog.Logger {
	return observability.NewLogger(os.Stdout, level, observability.DefaultFields()...)
}
