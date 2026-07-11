// Package app defines the wiring contract between the composition root
// (cmd/api) and the business modules.
//
// Deps carries the platform singletons a module needs; a module is constructed
// with those Deps and exposes an HTTP handler to mount and the event
// subscriptions to register. Cross-module access happens only through a
// module's public ports package — a module's domain/service/adapters live under
// its internal/ directory, so the Go compiler forbids other modules from
// importing them.
package app

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/audit"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Deps are the platform dependencies handed to every module at construction.
type Deps struct {
	Config     config.Config
	Pool       *pgxpool.Pool
	UnitOfWork *postgres.UnitOfWork
	Outbox     *events.Outbox
	Bus        *events.Bus
	Jobs       *jobs.Runner
	Audit      *audit.Writer
	Logger     *slog.Logger
	Clock      clock.Clock
	IDs        id.Generator
}

// Subscription binds an event type to a handler; the composition root registers
// each on the bus.
type Subscription struct {
	EventType string
	Handler   events.Handler
}

// Module is the contract every business module implements. The composition root
// mounts Handler under /api/v1/<Name> and registers Subscriptions on the bus.
type Module interface {
	Name() string
	Handler() http.Handler
	Subscriptions() []Subscription
}
