// Package example is a minimal reference module demonstrating the wiring
// pattern end-to-end: a driving HTTP adapter, a service owning the transaction
// boundary, a driven Postgres adapter, a published event, and an idempotent
// consumer of that event.
//
// It exists as living documentation. Delete this package and its migration when
// starting a real SaaS (see docs "Starting a new SaaS from this skeleton").
package example

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/example/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/example/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/example/internal/service"
	"github.com/williamokano/entitlements/internal/modules/example/ports"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// consumerName identifies this module's event consumer for idempotency.
const consumerName = "example-processor"

// Module wires the example module from platform dependencies.
type Module struct {
	deps    app.Deps
	svc     *service.Service
	handler http.Handler
}

// New constructs the example module.
func New(deps app.Deps) *Module {
	repo := pgadapter.New(deps.Pool)
	svc := service.New(deps.UnitOfWork, deps.Outbox, repo, deps.IDs, deps.Clock)
	return &Module{
		deps:    deps,
		svc:     svc,
		handler: rest.New(svc),
	}
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "example" }

// Handler is the module's HTTP handler, mounted under /api/v1/example.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions registers the idempotent consumer of example.thing_created,
// which marks the thing processed exactly once.
func (m *Module) Subscriptions() []app.Subscription {
	handler := events.Idempotent(consumerName, m.deps.Pool, func(ctx context.Context, e events.Event) error {
		var payload ports.ThingCreated
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return err
		}
		return m.svc.MarkProcessed(ctx, payload.ThingID)
	})
	return []app.Subscription{{EventType: ports.EventThingCreated, Handler: handler}}
}

// Port returns the module's public facade for other modules.
func (m *Module) Port() ports.Reader { return m.svc }
