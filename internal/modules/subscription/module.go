// Package subscription owns the subscription lifecycle: a tenant's commitment to
// a pinned plan version, driven by an explicit state machine.
package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/subscription/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/service"
	"github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/jobs"
)

// renewalInterval and trialInterval are how often the background scans run; the
// per-subscription due-ness is decided against the clock, so a coarse interval
// is fine.
const (
	renewalInterval = time.Minute
	trialInterval   = time.Minute
	graceInterval   = time.Minute
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
func New(deps app.Deps, catalog service.CatalogReader) *Module {
	svc := service.New(deps.UnitOfWork, deps.Outbox, pgadapter.New(deps.Pool), catalog, deps.IDs, deps.Clock,
		service.Config{BillingDisabled: deps.Config.BillingDisabled, TrialEndingDays: deps.Config.TrialEndingDays})
	return &Module{deps: deps, svc: svc, handler: rest.New(svc)}
}

// ApplyScheduledChange applies a subscription's scheduled plan change (no-op if
// none is pending). It is the period-boundary hook the renewal job (T-021)
// invokes at rollover.
func (m *Module) ApplyScheduledChange(ctx context.Context, subscriptionID uuid.UUID) error {
	return m.svc.ApplyScheduledChange(ctx, subscriptionID)
}

// RegisterJobs registers the recurring renewal and trial scans on the runner.
// The composition root calls this; tests may call it against their own runner.
func (m *Module) RegisterJobs(runner *jobs.Runner) error {
	if err := runner.Register("subscription.renewal", renewalInterval, func(ctx context.Context) error {
		_, err := m.svc.ProcessDueRenewals(ctx)
		return err
	}); err != nil {
		return err
	}
	if err := runner.Register("subscription.trial", trialInterval, func(ctx context.Context) error {
		_, _, err := m.svc.ProcessTrials(ctx)
		return err
	}); err != nil {
		return err
	}
	return runner.Register("subscription.grace", graceInterval, func(ctx context.Context) error {
		_, err := m.svc.ProcessGraceExpiries(ctx)
		return err
	})
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "subscription" }

// Handler is the module's HTTP handler, mounted under /api/v1/subscription.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions makes the module advance a subscription's period when its
// invoice is paid. The handler is idempotent (processed-events guarded), so a
// redelivered InvoicePaid never double-advances.
func (m *Module) Subscriptions() []app.Subscription {
	invoicePaid := events.Idempotent("subscription", m.deps.Pool, func(ctx context.Context, e events.Event) error {
		var p ports.BillingInvoicePaid
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		return m.svc.AdvancePeriodAfterPayment(ctx, p.SubscriptionID)
	})
	// Dunning drives the subscription lifecycle: a failed charge → past_due, an
	// exhausted schedule → grace, and a recovered payment → active.
	paymentFailed := m.dunningConsumer(func(ctx context.Context, subscriptionID uuid.UUID) error {
		return m.svc.MarkPaymentFailed(ctx, subscriptionID)
	})
	dunningExhausted := m.dunningConsumer(func(ctx context.Context, subscriptionID uuid.UUID) error {
		return m.svc.EnterGraceOnDunningExhausted(ctx, subscriptionID)
	})
	paymentRecovered := m.dunningConsumer(func(ctx context.Context, subscriptionID uuid.UUID) error {
		return m.svc.RecoverPayment(ctx, subscriptionID)
	})
	return []app.Subscription{
		{EventType: ports.EventBillingInvoicePaid, Handler: invoicePaid},
		{EventType: ports.EventBillingPaymentFailed, Handler: paymentFailed},
		{EventType: ports.EventBillingDunningExhausted, Handler: dunningExhausted},
		{EventType: ports.EventBillingPaymentRecovered, Handler: paymentRecovered},
	}
}

// dunningConsumer wraps a subscription-side dunning handler as an idempotent
// consumer that decodes the shared dunning payload (a subscription id) and
// routes it to fn.
func (m *Module) dunningConsumer(fn func(ctx context.Context, subscriptionID uuid.UUID) error) events.Handler {
	return events.Idempotent("subscription", m.deps.Pool, func(ctx context.Context, e events.Event) error {
		var p ports.BillingDunningEvent
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		return fn(ctx, p.SubscriptionID)
	})
}

// Port returns the subscription reader other modules use.
func (m *Module) Port() ports.SubscriptionReader { return m.svc }
