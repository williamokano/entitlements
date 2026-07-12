// Package subscription owns the subscription lifecycle: a tenant's commitment to
// a pinned plan version, driven by an explicit state machine.
package subscription

import (
	"net/http"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/subscription/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/service"
	"github.com/williamokano/entitlements/internal/modules/subscription/ports"
)

// Module wires the subscription module from platform dependencies and the
// catalog reader (subscriptions pin plan versions through it).
type Module struct {
	deps    app.Deps
	svc     *service.Service
	handler http.Handler
}

// New constructs the subscription module. catalog is the slice of the catalog it
// reads (the catalog module's port satisfies it).
func New(deps app.Deps, catalog service.PlanVersionReader) *Module {
	svc := service.New(deps.UnitOfWork, deps.Outbox, pgadapter.New(deps.Pool), catalog, deps.IDs, deps.Clock)
	return &Module{deps: deps, svc: svc, handler: rest.New(svc)}
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "subscription" }

// Handler is the module's HTTP handler, mounted under /api/v1/subscription.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions is empty: the subscription module publishes events but consumes
// none (yet).
func (m *Module) Subscriptions() []app.Subscription { return nil }

// Port returns the subscription reader other modules use.
func (m *Module) Port() ports.SubscriptionReader { return m.svc }
