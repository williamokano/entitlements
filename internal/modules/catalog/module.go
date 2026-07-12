// Package catalog is the product-catalog module: plans and immutable plan
// versions with per-cycle pricing, trial/grace config, and feature grants. The
// catalog is global (the SaaS operator's offering), not tenant-scoped.
package catalog

import (
	"net/http"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/catalog/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/catalog/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/catalog/internal/service"
	"github.com/williamokano/entitlements/internal/modules/catalog/ports"
)

// Module wires the catalog module from platform dependencies.
type Module struct {
	deps    app.Deps
	svc     *service.Service
	handler http.Handler
}

// New constructs the catalog module.
func New(deps app.Deps) *Module {
	svc := service.New(
		deps.UnitOfWork,
		deps.Outbox,
		pgadapter.NewPlans(deps.Pool),
		pgadapter.NewVersions(deps.Pool),
		deps.IDs,
		deps.Clock,
	)
	return &Module{deps: deps, svc: svc, handler: rest.New(svc)}
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "catalog" }

// Handler is the module's HTTP handler, mounted under /api/v1/catalog.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions is empty: the catalog publishes events but consumes none.
func (m *Module) Subscriptions() []app.Subscription { return nil }

// Port returns the catalog reader other modules use (e.g. a subscription pins a
// plan version through it).
func (m *Module) Port() ports.CatalogReader { return m.svc }
