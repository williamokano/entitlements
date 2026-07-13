//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/billing"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/fakeprovider"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
	billingports "github.com/williamokano/entitlements/internal/modules/billing/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

var billingEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// billingEnabledDeps builds deps with billing enabled (BILLING_DISABLED=false)
// so the billing module registers its renewal charge consumer. The clock is the
// system clock; tests that drive the consumer directly do not depend on time.
func billingEnabledDeps(t *testing.T) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	clk := clock.System
	return app.Deps{
		Config:     config.Config{BillingDisabled: false, TrialEndingDays: 3},
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        events.NewBus(),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
}

// frozenBillingDeps builds deps around a caller-controlled frozen clock, with
// billing enabled, for the full renewal→charge→invoice_paid→advance loop.
func frozenBillingDeps(t *testing.T, clk clock.Clock) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	return app.Deps{
		Config:     config.Config{BillingDisabled: false, TrialEndingDays: 3},
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        events.NewBus(),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
}

// renewalHandler returns the billing module's SubscriptionRenewalDue consumer.
func renewalHandler(t *testing.T, mod *billing.Module) events.Handler {
	t.Helper()
	subs := mod.Subscriptions()
	if len(subs) == 0 {
		t.Fatal("billing module exposes no subscriptions (billing must be enabled)")
	}
	if subs[0].EventType != subports.EventSubscriptionRenewalDue {
		t.Fatalf("unexpected subscription %q", subs[0].EventType)
	}
	return subs[0].Handler
}

// renewalEvent builds a delivered SubscriptionRenewalDue event.
func renewalEvent(tenantID, subID uuid.UUID) events.Event {
	payload, _ := json.Marshal(subports.SubscriptionRenewalDue{
		SubscriptionID: subID,
		TenantID:       tenantID,
		PlanVersionID:  uuid.New(),
		BillingCycle:   "monthly",
		PeriodEnd:      billingEpoch,
	})
	return events.Event{
		ID:         uuid.New(),
		OccurredAt: billingEpoch,
		TenantID:   tenantID,
		Module:     "subscription",
		Type:       subports.EventSubscriptionRenewalDue,
		Payload:    payload,
	}
}

func outboxCount(t *testing.T, deps app.Deps, tenantID uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = $2`, tenantID, eventType).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", eventType, err)
	}
	return n
}

func invoiceCount(t *testing.T, deps app.Deps, tenantID uuid.UUID) int {
	t.Helper()
	var n int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM billing.invoices WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil {
		t.Fatalf("count invoices: %v", err)
	}
	return n
}

func latestInvoice(t *testing.T, deps app.Deps, tenantID uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	var (
		invID  uuid.UUID
		status string
	)
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT id, status FROM billing.invoices WHERE tenant_id = $1 ORDER BY number DESC LIMIT 1`, tenantID).
		Scan(&invID, &status); err != nil {
		t.Fatalf("load invoice: %v", err)
	}
	return invID, status
}

func subPeriodEnd(t *testing.T, deps app.Deps, tenantID uuid.UUID) time.Time {
	t.Helper()
	var end time.Time
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT current_period_end FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&end); err != nil {
		t.Fatalf("load subscription period: %v", err)
	}
	return end
}

// TestWebhookTranslationNormalizesProviderEvent proves a raw provider webhook is
// normalized into a provider-agnostic ProviderEvent.
func TestWebhookTranslationNormalizesProviderEvent(t *testing.T) {
	p := fakeprovider.New()
	inv := uuid.New()
	body := []byte(fmt.Sprintf(
		`{"event":"payment.succeeded","charge":"ch_1","invoice":"%s","amount_minor":2500,"currency":"USD"}`, inv))

	ev, err := p.TranslateWebhook(context.Background(), service.WebhookInput{Type: "payment.succeeded", Body: body})
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if ev.Kind != service.ProviderEventChargeSucceeded {
		t.Fatalf("kind = %q, want %q", ev.Kind, service.ProviderEventChargeSucceeded)
	}
	if ev.InvoiceID != inv {
		t.Fatalf("invoice = %s, want %s", ev.InvoiceID, inv)
	}
	if ev.AmountMinor != 2500 || ev.Currency != "USD" || ev.ProviderRef != "ch_1" {
		t.Fatalf("normalized fields wrong: %+v", ev)
	}

	failed, err := p.TranslateWebhook(context.Background(), service.WebhookInput{Body: []byte(`{"event":"payment.failed","charge":"ch_2"}`)})
	if err != nil || failed.Kind != service.ProviderEventChargeFailed {
		t.Fatalf("failed webhook = %+v err=%v", failed, err)
	}
	unknown, err := p.TranslateWebhook(context.Background(), service.WebhookInput{Body: []byte(`{"event":"something.else"}`)})
	if err != nil || unknown.Kind != service.ProviderEventUnknown {
		t.Fatalf("unknown webhook = %+v err=%v", unknown, err)
	}
}

// TestPaymentMethodStoresTokenOnlyNeverPAN proves the service/domain guard and
// the schema CHECK both refuse a raw card number; only token references persist.
func TestPaymentMethodStoresTokenOnlyNeverPAN(t *testing.T) {
	deps := billingEnabledDeps(t)
	cat := defaultCatalog()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly"}
	fake := fakeprovider.New()
	mod := billing.New(deps, cat, subs, billing.WithPaymentProvider(fake))
	tenantID := uuid.New()
	ctx := ctxFor(tenantID)

	// A provider token stores fine.
	pm, err := mod.AttachPaymentMethod(ctx, "pm_visa_4242", "visa", "4242")
	if err != nil {
		t.Fatalf("attach token: %v", err)
	}
	if pm.Token != "pm_visa_4242" {
		t.Fatalf("stored token = %q", pm.Token)
	}

	// A raw PAN (bare and separated) is rejected before anything is stored.
	for _, pan := range []string{"4242424242424242", "4242 4242 4242 4242", "4242-4242-4242-4242"} {
		if _, err := mod.AttachPaymentMethod(ctx, pan, "visa", "4242"); err == nil {
			t.Fatalf("raw PAN %q was accepted; must be rejected", pan)
		}
	}

	// Exactly one (tokenized) payment method persisted.
	list, err := mod.ListPaymentMethods(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Token != "pm_visa_4242" {
		t.Fatalf("stored methods = %+v, want one token", list)
	}

	// Schema backstop: a direct INSERT of a PAN is rejected by the CHECK.
	if _, err := deps.Pool.Exec(context.Background(),
		`INSERT INTO billing.payment_methods (id, tenant_id, customer_ref, token, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		uuid.New(), tenantID, "cus_x", "4111111111111111"); err == nil {
		t.Fatal("schema accepted a raw PAN; token_not_pan CHECK is missing")
	}
}

// TestRenewalFlowChargeFailsEmitsPaymentFailed drives the renewal consumer with a
// declining provider: the invoice is issued and stays open, and payment_failed is
// emitted (invoice_paid is not).
func TestRenewalFlowChargeFailsEmitsPaymentFailed(t *testing.T) {
	deps := billingEnabledDeps(t)
	cat := defaultCatalog()
	tenantID, subID := uuid.New(), uuid.New()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly", subByTenant: map[uuid.UUID]uuid.UUID{tenantID: subID}}
	fake := fakeprovider.NewWith(fakeprovider.WithChargeFailure(-1, "card_declined"))
	mod := billing.New(deps, cat, subs, billing.WithPaymentProvider(fake))

	handler := renewalHandler(t, mod)
	if err := handler(context.Background(), renewalEvent(tenantID, subID)); err != nil {
		t.Fatalf("renewal handler: %v", err)
	}

	if _, status := latestInvoice(t, deps, tenantID); status != "open" {
		t.Fatalf("invoice status = %s, want open (charge failed)", status)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventPaymentFailed); n != 1 {
		t.Fatalf("payment_failed events = %d, want 1", n)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventInvoicePaid); n != 0 {
		t.Fatalf("invoice_paid events = %d, want 0", n)
	}
	if fake.ChargeCount() != 1 {
		t.Fatalf("charges = %d, want 1", fake.ChargeCount())
	}
}

// TestDuplicateRenewalDueDeliveryProducesExactlyOneCharge redelivers the same
// renewal event twice; the idempotent consumer drops the duplicate, so the fake
// records exactly one charge and one invoice is issued.
func TestDuplicateRenewalDueDeliveryProducesExactlyOneCharge(t *testing.T) {
	deps := billingEnabledDeps(t)
	cat := defaultCatalog()
	tenantID, subID := uuid.New(), uuid.New()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly", subByTenant: map[uuid.UUID]uuid.UUID{tenantID: subID}}
	fake := fakeprovider.New()
	mod := billing.New(deps, cat, subs, billing.WithPaymentProvider(fake))

	handler := renewalHandler(t, mod)
	e := renewalEvent(tenantID, subID) // same event ID both deliveries
	for i := 0; i < 2; i++ {
		if err := handler(context.Background(), e); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}

	if fake.ChargeCount() != 1 {
		t.Fatalf("charges = %d, want exactly 1 (idempotent consumer)", fake.ChargeCount())
	}
	if n := invoiceCount(t, deps, tenantID); n != 1 {
		t.Fatalf("invoices = %d, want 1", n)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventInvoicePaid); n != 1 {
		t.Fatalf("invoice_paid events = %d, want 1", n)
	}
}

// TestProviderChargeReceivesStableIdempotencyKey proves the provider is charged
// with a key derived from (invoice, attempt), and that key is stable — a
// redelivery does not re-charge, and the derivation is deterministic.
func TestProviderChargeReceivesStableIdempotencyKey(t *testing.T) {
	deps := billingEnabledDeps(t)
	cat := defaultCatalog()
	tenantID, subID := uuid.New(), uuid.New()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly", subByTenant: map[uuid.UUID]uuid.UUID{tenantID: subID}}
	fake := fakeprovider.New()
	mod := billing.New(deps, cat, subs, billing.WithPaymentProvider(fake))

	handler := renewalHandler(t, mod)
	e := renewalEvent(tenantID, subID)
	if err := handler(context.Background(), e); err != nil {
		t.Fatalf("renewal handler: %v", err)
	}

	invID, _ := latestInvoice(t, deps, tenantID)
	want := service.ChargeIdempotencyKey(invID, 1)
	charges := fake.Charges()
	if len(charges) != 1 {
		t.Fatalf("charges = %d, want 1", len(charges))
	}
	if charges[0].IdempotencyKey != want {
		t.Fatalf("charge key = %q, want %q", charges[0].IdempotencyKey, want)
	}

	// Redeliver the same renewal: no second charge (idempotent), key unchanged.
	if err := handler(context.Background(), e); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if fake.ChargeCount() != 1 {
		t.Fatalf("redelivery re-charged: count = %d", fake.ChargeCount())
	}
	if k := service.ChargeIdempotencyKey(invID, 1); k != want {
		t.Fatalf("key not deterministic: %q vs %q", k, want)
	}
}

// TestRenewalFlowChargeSucceedsInvoicePaidPeriodAdvances is the full loop with
// the real subscription module: renewal_due → billing issues + charges (fake
// succeeds) → invoice_paid → subscription advances its period.
func TestRenewalFlowChargeSucceedsInvoicePaidPeriodAdvances(t *testing.T) {
	clk := clock.NewFrozen(billingEpoch)
	deps := frozenBillingDeps(t, clk)
	cat := defaultCatalog()

	subMod := subscription.New(deps, cat)
	fake := fakeprovider.New()
	billMod := billing.New(deps, cat, subMod.Port(), billing.WithPaymentProvider(fake))

	for _, s := range subMod.Subscriptions() {
		deps.Bus.Subscribe(s.EventType, s.Handler)
	}
	for _, s := range billMod.Subscriptions() {
		deps.Bus.Subscribe(s.EventType, s.Handler)
	}

	runner := jobs.NewRunner(deps.Pool, deps.IDs, clk, deps.Logger, time.Second)
	if err := subMod.RegisterJobs(runner); err != nil {
		t.Fatalf("register jobs: %v", err)
	}
	relay := events.NewRelay(deps.Pool, deps.Bus, clk, deps.Logger, events.RelayConfig{})

	tenantID := uuid.New()
	srv := serverFor(subMod.Handler(), tenantID)
	defer srv.Close()
	if code, body := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); code != 201 {
		t.Fatalf("create subscription = %d %s", code, body)
	}

	endBefore := subPeriodEnd(t, deps, tenantID)

	// Advance past the period end; the renewal job emits renewal_due (no
	// auto-advance, because billing is enabled).
	clk.Set(billingEpoch.AddDate(0, 1, 2))
	ctx := context.Background()
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("run renewal job: %v", err)
	}
	if n := outboxCount(t, deps, tenantID, subports.EventSubscriptionRenewalDue); n != 1 {
		t.Fatalf("renewal_due = %d, want 1", n)
	}

	// Drain the outbox: renewal_due → charge → invoice_paid → advance.
	if _, err := relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay process: %v", err)
	}

	endAfter := subPeriodEnd(t, deps, tenantID)
	if !endAfter.After(endBefore) {
		t.Fatalf("period not advanced via charge flow: before=%v after=%v", endBefore, endAfter)
	}
	if fake.ChargeCount() != 1 {
		t.Fatalf("charges = %d, want 1", fake.ChargeCount())
	}
	if _, status := latestInvoice(t, deps, tenantID); status != "paid" {
		t.Fatalf("invoice status = %s, want paid", status)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventInvoicePaid); n != 1 {
		t.Fatalf("invoice_paid = %d, want 1", n)
	}
}
