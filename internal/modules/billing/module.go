// Package billing owns the money side: invoices generated from a subscription
// with line-item snapshots, a guarded invoice lifecycle state machine, per-tenant
// gapless invoice numbering, credit notes, and the charge flow — an abstract
// PaymentProvider port (with a fake adapter for the skeleton), tokenized
// payment-method storage, and an idempotent consumer of SubscriptionRenewalDue
// that issues an invoice, charges the provider, and reports the outcome as
// billing.invoice_paid (subscription advances its period) or
// billing.payment_failed. An issued invoice is a historical fact — its lines copy
// the plan/addon key, version, unit price, quantity and currency at issuance, so
// later catalog changes never rewrite it. Dunning + proration are T-027.
package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/fakeprovider"
	pgadapter "github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
	"github.com/williamokano/entitlements/internal/modules/billing/ports"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/jobs"
)

// dunningRetryInterval is how often the background dunning scan wakes; per-retry
// due-ness is decided against the clock, so a coarse interval is fine.
const dunningRetryInterval = time.Minute

// defaultDunningOffsets is the fallback retry schedule when none is configured
// (or the configured value fails to parse): retry 1, 3 and 7 days after the
// initial failure.
var defaultDunningOffsets = []time.Duration{
	1 * 24 * time.Hour,
	3 * 24 * time.Hour,
	7 * 24 * time.Hour,
}

// Module wires the billing module from platform dependencies and the catalog +
// subscription readers (invoices snapshot the pinned plan/addon versions and read
// the tenant's live subscription through them).
type Module struct {
	deps    app.Deps
	svc     *service.Service
	handler http.Handler
}

// Option customizes the billing module at construction.
type Option func(*config)

type config struct {
	provider service.PaymentProvider
}

// WithPaymentProvider overrides the default fake payment provider with a real
// gateway adapter (Stripe/Adyen/etc). A production SaaS injects its provider
// here and sets BILLING_DISABLED=false to run the real charge flow.
func WithPaymentProvider(p service.PaymentProvider) Option {
	return func(c *config) { c.provider = p }
}

// New constructs the billing module. catalog and subs are the slices of the
// catalog and subscription modules billing reads; their ports satisfy them. Tax
// defaults to the no-op calculator (zero tax); the payment provider defaults to
// the shipped in-memory fake (auto-succeeds) unless overridden with
// WithPaymentProvider.
func New(deps app.Deps, catalog service.CatalogReader, subs service.SubscriptionReader, opts ...Option) *Module {
	cfg := config{provider: fakeprovider.New()}
	for _, o := range opts {
		o(&cfg)
	}
	offsets := parseDunningOffsets(deps.Config.DunningOffsets)
	proration := service.NewProrationStrategy(deps.Config.ProrationStrategy)
	svc := service.New(deps.UnitOfWork, deps.Outbox, pgadapter.New(deps.Pool), catalog, subs,
		service.NoopTaxCalculator{}, cfg.provider, proration, offsets, deps.IDs, deps.Clock)
	return &Module{deps: deps, svc: svc, handler: rest.New(svc)}
}

// parseDunningOffsets turns the configured schedule (Go durations or "Nd" day
// counts, an optional leading "+" allowed) into offsets from the initial
// failure. An empty or unparseable configuration falls back to the default
// 1d,3d,7d schedule so the module always has a sane retry policy.
func parseDunningOffsets(specs []string) []time.Duration {
	if len(specs) == 0 {
		return defaultDunningOffsets
	}
	out := make([]time.Duration, 0, len(specs))
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		spec = strings.TrimPrefix(spec, "+")
		var (
			d   time.Duration
			err error
		)
		if days, ok := strings.CutSuffix(spec, "d"); ok {
			n, convErr := strconv.Atoi(days)
			if convErr != nil {
				return defaultDunningOffsets
			}
			d = time.Duration(n) * 24 * time.Hour
		} else if d, err = time.ParseDuration(spec); err != nil {
			return defaultDunningOffsets
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return defaultDunningOffsets
	}
	return out
}

// RegisterJobs registers the recurring dunning retry scan on the runner (when
// billing is enabled). The composition root calls this; tests may call it
// against their own runner.
func (m *Module) RegisterJobs(runner *jobs.Runner) error {
	if m.deps.Config.BillingDisabled {
		return nil
	}
	return runner.Register("billing.dunning", dunningRetryInterval, func(ctx context.Context) error {
		_, err := m.svc.ProcessDueDunning(ctx)
		return err
	})
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "billing" }

// Handler is the module's HTTP handler, mounted under /api/v1/billing.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions wires the renewal charge flow: when billing is enabled, billing
// consumes SubscriptionRenewalDue and drives issue → charge → invoice_paid |
// payment_failed. The consumer is idempotent (processed-events guarded), so a
// redelivered renewal never issues a second invoice or a second charge.
//
// When BILLING_DISABLED is true (the default), billing does not subscribe:
// the subscription module auto-advances the period on renewal instead, so the
// two paths never both advance a period.
func (m *Module) Subscriptions() []app.Subscription {
	if m.deps.Config.BillingDisabled {
		return nil
	}
	renewal := events.Idempotent("billing", m.deps.Pool, func(ctx context.Context, e events.Event) error {
		var p subports.SubscriptionRenewalDue
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		return m.svc.ChargeForRenewal(ctx, p.TenantID)
	})
	// A mid-period plan change is prorated into an invoice line (or a deferred
	// pending proration) by the configured strategy.
	planChanged := events.Idempotent("billing", m.deps.Pool, func(ctx context.Context, e events.Event) error {
		var p subports.SubscriptionPlanChanged
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		return m.svc.ApplyPlanChange(ctx, p.TenantID, p.SubscriptionID, p.OldPlanVersionID, p.NewPlanVersionID, p.OldCycle, p.NewCycle)
	})
	return []app.Subscription{
		{EventType: subports.EventSubscriptionRenewalDue, Handler: renewal},
		{EventType: subports.EventSubscriptionPlanChanged, Handler: planChanged},
	}
}

// AttachPaymentMethod tokenizes and stores a payment method for the tenant in
// ctx, persisting tokens only (a raw PAN is rejected before storage). It is the
// composition root's / caller's entry point for adding a card; there is no HTTP
// surface for it in this task.
func (m *Module) AttachPaymentMethod(ctx context.Context, token, brand, last4 string) (service.PaymentMethodView, error) {
	return m.svc.AttachPaymentMethod(ctx, token, brand, last4)
}

// ListPaymentMethods returns the tenant's stored (tokenized) payment methods.
func (m *Module) ListPaymentMethods(ctx context.Context) ([]service.PaymentMethodView, error) {
	return m.svc.ListPaymentMethods(ctx)
}

// Port returns the billing reader other modules use.
func (m *Module) Port() ports.BillingReader { return m.svc }
