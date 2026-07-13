// Package entitlements is the product core: a dynamic feature registry plus the
// resolution pipeline that composes plan grants, addon deltas (× quantity), and
// tenant overrides into a tenant's effective entitlements. It materializes the
// set per tenant, rebuilds it from subscription events, and publishes an
// EntitlementsSummaryChanged whenever the set changes.
package entitlements

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/service"
	"github.com/williamokano/entitlements/internal/modules/entitlements/ports"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// Module wires the entitlements module from platform dependencies plus the
// catalog and subscription readers the resolver composes.
type Module struct {
	deps    app.Deps
	svc     *service.Service
	handler http.Handler
}

// New constructs the entitlements module. catalog and subs are the slices of the
// catalog and subscription modules the resolver reads (each module's port
// satisfies the corresponding interface).
func New(deps app.Deps, catalog service.CatalogReader, subs service.SubscriptionReader) *Module {
	policy := domain.UnknownDeny
	if deps.Config.EntitlementsUnknownFeaturePolicy == "allow" {
		policy = domain.UnknownAllow
	}
	svc := service.New(deps.UnitOfWork, deps.Outbox, postgres.New(deps.Pool), catalog, subs, deps.IDs, deps.Clock,
		service.Config{UnknownFeaturePolicy: policy})
	return &Module{deps: deps, svc: svc, handler: rest.New(svc)}
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "entitlements" }

// Handler is the module's HTTP handler, mounted under /api/v1/entitlements.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions re-materializes a tenant's effective set whenever its
// subscription transitions, changes plan, or changes an addon. Each handler is
// wrapped in events.Idempotent so a redelivered event converges (and
// materialization itself is a no-op when nothing changed).
func (m *Module) Subscriptions() []app.Subscription {
	rematerialize := func(extract func(events.Event) (uuid.UUID, error)) events.Handler {
		return events.Idempotent("entitlements", m.deps.Pool, func(ctx context.Context, e events.Event) error {
			tenantID, err := extract(e)
			if err != nil {
				return err
			}
			return m.svc.Materialize(ctx, tenantID)
		})
	}
	return []app.Subscription{
		{EventType: subports.EventSubscriptionTransitioned, Handler: rematerialize(func(e events.Event) (uuid.UUID, error) {
			var p subports.SubscriptionTransitioned
			err := json.Unmarshal(e.Payload, &p)
			return p.TenantID, err
		})},
		{EventType: subports.EventSubscriptionPlanChanged, Handler: rematerialize(func(e events.Event) (uuid.UUID, error) {
			var p subports.SubscriptionPlanChanged
			err := json.Unmarshal(e.Payload, &p)
			return p.TenantID, err
		})},
		{EventType: subports.EventSubscriptionAddonChanged, Handler: rematerialize(func(e events.Event) (uuid.UUID, error) {
			var p subports.SubscriptionAddonChanged
			err := json.Unmarshal(e.Payload, &p)
			return p.TenantID, err
		})},
	}
}

// Port returns the entitlements reader other modules use.
func (m *Module) Port() ports.EntitlementsReader { return m.svc }

// Service exposes the use cases for the composition root's provisioning/seeding
// and for tests (feature-registry CRUD, Resolve, Materialize).
func (m *Module) Service() *service.Service { return m.svc }

// Materialize re-resolves and rebuilds a tenant's effective set, publishing an
// EntitlementsSummaryChanged when it changes. It is exported so provisioning
// hooks and tests can drive materialization directly.
func (m *Module) Materialize(ctx context.Context, tenantID uuid.UUID) error {
	return m.svc.Materialize(ctx, tenantID)
}
