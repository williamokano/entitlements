// Package billing owns the money side: invoices generated from a subscription
// with line-item snapshots, a guarded invoice lifecycle state machine, per-tenant
// gapless invoice numbering, and credit notes. An issued invoice is a historical
// fact — its lines copy the plan/addon key, version, unit price, quantity and
// currency at issuance, so later catalog changes never rewrite it. Billing reports
// money outcomes as events (billing.invoice_paid) that subscription reacts to; the
// PaymentProvider/charge flow and dunning are separate tasks (T-026/T-027).
package billing

import (
	"net/http"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
	"github.com/williamokano/entitlements/internal/modules/billing/ports"
)

// Module wires the billing module from platform dependencies and the catalog +
// subscription readers (invoices snapshot the pinned plan/addon versions and read
// the tenant's live subscription through them).
type Module struct {
	svc     *service.Service
	handler http.Handler
}

// New constructs the billing module. catalog and subs are the slices of the
// catalog and subscription modules billing reads; their ports satisfy them. Tax
// defaults to the no-op calculator (zero tax).
func New(deps app.Deps, catalog service.CatalogReader, subs service.SubscriptionReader) *Module {
	svc := service.New(deps.UnitOfWork, deps.Outbox, pgadapter.New(deps.Pool), catalog, subs, service.NoopTaxCalculator{}, deps.IDs, deps.Clock)
	return &Module{svc: svc, handler: rest.New(svc)}
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "billing" }

// Handler is the module's HTTP handler, mounted under /api/v1/billing.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions returns the module's event subscriptions. Billing consumes
// SubscriptionRenewalDue in the charge flow (T-026); it has none here.
func (m *Module) Subscriptions() []app.Subscription { return nil }

// Port returns the billing reader other modules use.
func (m *Module) Port() ports.BillingReader { return m.svc }
