package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/example"
	"github.com/williamokano/entitlements/internal/modules/tenant"
	"github.com/williamokano/entitlements/internal/platform/audit"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/httpx"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
	"github.com/williamokano/entitlements/internal/platform/observability"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// idempotencyTTL bounds how long a stored idempotent response is replayable.
const idempotencyTTL = 24 * time.Hour

// application is the assembled monolith: the HTTP handler plus the background
// workers (outbox relay and job runner).
type application struct {
	handler http.Handler
	relay   *events.Relay
	jobs    *jobs.Runner
	pool    *pgxpool.Pool
}

// buildApplication constructs platform dependencies, wires every module, and
// assembles the HTTP handler and background workers. It is separate from run so
// tests can drive the wired handler without binding a socket.
func buildApplication(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) *application {
	clk := clock.System
	ids := id.UUIDv7{}
	bus := events.NewBus()

	deps := app.Deps{
		Config:     cfg,
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        bus,
		Jobs:       jobs.NewRunner(pool, ids, clk, logger, time.Second),
		Audit:      audit.NewWriter(pool, ids, clk),
		Logger:     logger,
		Clock:      clk,
		IDs:        ids,
	}

	// The set of business modules. New modules are added here. Each SaaS
	// registers its tenant provisioning hooks (seed roles, create a trial
	// subscription, …) alongside the default logging hook.
	modules := []app.Module{
		tenant.New(deps, tenant.WithProvisioningHooks(tenant.NewLoggingHook(logger))),
		example.New(deps),
	}

	mws := []httpx.Middleware{
		httpx.RequestID(ids),
		observability.TracingMiddleware(otel.GetTracerProvider()),
		httpx.Recovery(logger),
		httpx.Logging(logger),
		httpx.Idempotency(pool, clk, idempotencyTTL),
	}
	router := httpx.NewRouter(mws...)
	router.HandleFunc("GET /healthz", httpx.Health())
	router.HandleFunc("GET /readyz", httpx.Health())

	for _, m := range modules {
		router.Mount("/api/v1/"+m.Name(), m.Handler())
		for _, sub := range m.Subscriptions() {
			bus.Subscribe(sub.EventType, sub.Handler)
		}
	}

	return &application{
		handler: router.Handler(),
		relay:   events.NewRelay(pool, bus, clk, logger, events.RelayConfig{}),
		jobs:    deps.Jobs,
		pool:    pool,
	}
}

// run is the full boot sequence: config → observability → Postgres → migrations
// → modules → background workers → serve, shutting down gracefully on ctx.
func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	shutdownOTel, err := observability.SetupOTel(ctx, observability.Options{
		ServiceName:  cfg.ServiceName,
		OTLPEndpoint: cfg.OTLPEndpoint,
	})
	if err != nil {
		return fmt.Errorf("setup opentelemetry: %w", err)
	}
	defer shutdownWithTimeout(shutdownOTel, logger)

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if err := postgres.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	a := buildApplication(cfg, pool, logger)

	// Background workers: the outbox relay and the scheduled-job runner.
	go func() {
		if err := a.relay.Run(ctx); err != nil {
			logger.Error("outbox relay stopped", "error", err)
		}
	}()
	go func() {
		if err := a.jobs.Run(ctx); err != nil {
			logger.Error("job runner stopped", "error", err)
		}
	}()

	logger.Info("starting api", "env", cfg.Environment)
	return httpx.NewServer(":"+cfg.HTTPPort, a.handler, logger).Run(ctx)
}

func shutdownWithTimeout(fn func(context.Context) error, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}
