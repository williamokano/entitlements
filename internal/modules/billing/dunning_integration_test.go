//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/billing"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/adapters/fakeprovider"
	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
	billingports "github.com/williamokano/entitlements/internal/modules/billing/ports"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/jobs"
)

const day = 24 * time.Hour

// --- helpers ---

func subStatus(t *testing.T, deps app.Deps, tenantID uuid.UUID) string {
	t.Helper()
	var s string
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT status FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&s); err != nil {
		t.Fatalf("load subscription status: %v", err)
	}
	return s
}

func graceEndsAt(t *testing.T, deps app.Deps, tenantID uuid.UUID) time.Time {
	t.Helper()
	var ge *time.Time
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT grace_ends_at FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&ge); err != nil {
		t.Fatalf("load grace_ends_at: %v", err)
	}
	if ge == nil {
		t.Fatal("grace_ends_at is null; subscription is not in grace")
	}
	return ge.UTC()
}

// planChangedHandler returns the billing module's SubscriptionPlanChanged consumer.
func planChangedHandler(t *testing.T, mod *billing.Module) events.Handler {
	t.Helper()
	for _, s := range mod.Subscriptions() {
		if s.EventType == subports.EventSubscriptionPlanChanged {
			return s.Handler
		}
	}
	t.Fatal("billing module exposes no plan-changed subscription")
	return nil
}

func planChangedEvent(tenantID, subID, oldPV, newPV uuid.UUID) events.Event {
	payload, _ := json.Marshal(subports.SubscriptionPlanChanged{
		SubscriptionID:   subID,
		TenantID:         tenantID,
		OldPlanVersionID: oldPV,
		NewPlanVersionID: newPV,
		OldCycle:         "monthly",
		NewCycle:         "monthly",
		Timing:           "immediate",
	})
	return events.Event{
		ID:         uuid.New(),
		OccurredAt: billingEpoch,
		TenantID:   tenantID,
		Module:     "subscription",
		Type:       subports.EventSubscriptionPlanChanged,
		Payload:    payload,
	}
}

// TestDunningRetriesAtConfiguredOffsets proves the dunning job retries the failed
// charge at exactly the configured offsets (+1d, +3d, +7d) and nowhere else, and
// then stops (exhausted).
func TestDunningRetriesAtConfiguredOffsets(t *testing.T) {
	clk := clock.NewFrozen(billingEpoch)
	deps := frozenBillingDeps(t, clk)
	cat := defaultCatalog()
	tenantID, subID := uuid.New(), uuid.New()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly", subByTenant: map[uuid.UUID]uuid.UUID{tenantID: subID}}
	fake := fakeprovider.NewWith(fakeprovider.WithChargeFailure(-1, "card_declined"))
	mod := billing.New(deps, cat, subs, billing.WithPaymentProvider(fake))

	ctx := context.Background()

	// Initial renewal at T0: the charge fails and dunning opens (attempt 1).
	if err := renewalHandler(t, mod)(ctx, renewalEvent(tenantID, subID)); err != nil {
		t.Fatalf("renewal handler: %v", err)
	}
	if fake.ChargeCount() != 1 {
		t.Fatalf("after initial failure charges = %d, want 1", fake.ChargeCount())
	}
	invID, status := latestInvoice(t, deps, tenantID)
	if status != "open" {
		t.Fatalf("invoice status = %s, want open", status)
	}

	runner := jobs.NewRunner(deps.Pool, deps.IDs, clk, deps.Logger, time.Second)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatalf("register billing jobs: %v", err)
	}

	// Walk the clock day by day. A retry must happen only at +1d, +3d, +7d.
	wantChargesAt := map[int]int{
		1: 2, // +1d: attempt 2
		2: 2, // +2d: no retry (schedule points at +3d)
		3: 3, // +3d: attempt 3
		4: 3,
		5: 3,
		6: 3,
		7: 4, // +7d: attempt 4 (final), then exhausted
		8: 4, // +8d: exhausted, no more retries
		9: 4,
	}
	for d := 1; d <= 9; d++ {
		clk.Set(billingEpoch.Add(time.Duration(d) * day))
		if err := runner.RunDue(ctx); err != nil {
			t.Fatalf("run dunning job at +%dd: %v", d, err)
		}
		if got := fake.ChargeCount(); got != wantChargesAt[d] {
			t.Fatalf("charges after +%dd = %d, want %d (retries only at +1d/+3d/+7d)", d, got, wantChargesAt[d])
		}
	}

	// Exactly one exhaustion event, and the charge keys are attempts 1..4.
	if n := outboxCount(t, deps, tenantID, billingports.EventDunningExhausted); n != 1 {
		t.Fatalf("dunning_exhausted events = %d, want 1", n)
	}
	charges := fake.Charges()
	if len(charges) != 4 {
		t.Fatalf("total charges = %d, want 4", len(charges))
	}
	for i, c := range charges {
		want := service.ChargeIdempotencyKey(invID, i+1)
		if c.IdempotencyKey != want {
			t.Fatalf("charge %d key = %q, want %q (new attempt per retry)", i, c.IdempotencyKey, want)
		}
	}
}

// TestPaymentRecoveredMidDunningStopsRetriesAndReactivates proves a retry that
// succeeds mid-schedule pays the invoice, stops further retries, and returns the
// subscription past_due → active.
func TestPaymentRecoveredMidDunningStopsRetriesAndReactivates(t *testing.T) {
	clk := clock.NewFrozen(billingEpoch)
	deps := frozenBillingDeps(t, clk)
	cat := defaultCatalog()

	subMod := subscription.New(deps, cat)
	// Fail only the initial renewal charge; the first dunning retry succeeds.
	fake := fakeprovider.NewWith(fakeprovider.WithChargeFailure(1, "card_declined"))
	billMod := billing.New(deps, cat, subMod.Port(), billing.WithPaymentProvider(fake))

	for _, s := range subMod.Subscriptions() {
		deps.Bus.Subscribe(s.EventType, s.Handler)
	}
	for _, s := range billMod.Subscriptions() {
		deps.Bus.Subscribe(s.EventType, s.Handler)
	}
	runner := jobs.NewRunner(deps.Pool, deps.IDs, clk, deps.Logger, time.Second)
	if err := subMod.RegisterJobs(runner); err != nil {
		t.Fatalf("register subscription jobs: %v", err)
	}
	if err := billMod.RegisterJobs(runner); err != nil {
		t.Fatalf("register billing jobs: %v", err)
	}
	relay := events.NewRelay(deps.Pool, deps.Bus, clk, deps.Logger, events.RelayConfig{})
	ctx := context.Background()

	tenantID := uuid.New()
	srv := serverFor(subMod.Handler(), tenantID)
	defer srv.Close()
	if code, body := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); code != 201 {
		t.Fatalf("create subscription = %d %s", code, body)
	}

	// Renewal: charge fails → payment_failed → past_due.
	clk.Set(billingEpoch.AddDate(0, 1, 2))
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("renewal tick: %v", err)
	}
	if _, err := relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay after renewal: %v", err)
	}
	if s := subStatus(t, deps, tenantID); s != "past_due" {
		t.Fatalf("status after failed charge = %s, want past_due", s)
	}
	if fake.ChargeCount() != 1 {
		t.Fatalf("charges after renewal = %d, want 1", fake.ChargeCount())
	}
	failedAt := clk.Now()

	// +1d: dunning retry succeeds → invoice paid, payment_recovered → active.
	clk.Set(failedAt.Add(day))
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("dunning tick: %v", err)
	}
	if _, err := relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay after retry: %v", err)
	}
	if s := subStatus(t, deps, tenantID); s != "active" {
		t.Fatalf("status after recovery = %s, want active", s)
	}
	if fake.ChargeCount() != 2 {
		t.Fatalf("charges after recovery = %d, want 2 (initial + one retry)", fake.ChargeCount())
	}
	if _, status := latestInvoice(t, deps, tenantID); status != "paid" {
		t.Fatalf("invoice status = %s, want paid", status)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventPaymentRecovered); n != 1 {
		t.Fatalf("payment_recovered events = %d, want 1", n)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventDunningExhausted); n != 0 {
		t.Fatalf("dunning_exhausted events = %d, want 0 (recovered)", n)
	}

	// Later ticks must not retry again — the schedule is recovered (terminal).
	clk.Set(failedAt.Add(5 * day))
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("post-recovery tick: %v", err)
	}
	if fake.ChargeCount() != 2 {
		t.Fatalf("charges after recovery settled = %d, want 2 (no further retries)", fake.ChargeCount())
	}
}

// TestDunningExhaustedDrivesGraceThenSuspended proves an exhausted dunning
// schedule moves the subscription past_due → grace with the grace length taken
// from the pinned plan version, then to suspended once the grace period elapses.
func TestDunningExhaustedDrivesGraceThenSuspended(t *testing.T) {
	const graceDays = 5
	clk := clock.NewFrozen(billingEpoch)
	deps := frozenBillingDeps(t, clk)
	// Plan version pins a 5-day grace period.
	cat := &fakeCatalog{
		planPV: catalogports.PlanVersionInfo{
			PlanKey: "pro", Version: 1, Status: "published", Currency: "USD",
			Prices:    []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 1000}},
			GraceDays: graceDays,
		},
		addons: map[uuid.UUID]catalogports.AddonVersionInfo{},
	}

	subMod := subscription.New(deps, cat)
	fake := fakeprovider.NewWith(fakeprovider.WithChargeFailure(-1, "card_declined"))
	billMod := billing.New(deps, cat, subMod.Port(), billing.WithPaymentProvider(fake))

	for _, s := range subMod.Subscriptions() {
		deps.Bus.Subscribe(s.EventType, s.Handler)
	}
	for _, s := range billMod.Subscriptions() {
		deps.Bus.Subscribe(s.EventType, s.Handler)
	}
	runner := jobs.NewRunner(deps.Pool, deps.IDs, clk, deps.Logger, time.Second)
	if err := subMod.RegisterJobs(runner); err != nil {
		t.Fatalf("register subscription jobs: %v", err)
	}
	if err := billMod.RegisterJobs(runner); err != nil {
		t.Fatalf("register billing jobs: %v", err)
	}
	relay := events.NewRelay(deps.Pool, deps.Bus, clk, deps.Logger, events.RelayConfig{})
	ctx := context.Background()

	tenantID := uuid.New()
	srv := serverFor(subMod.Handler(), tenantID)
	defer srv.Close()
	if code, body := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); code != 201 {
		t.Fatalf("create subscription = %d %s", code, body)
	}

	// Renewal: charge fails → past_due.
	clk.Set(billingEpoch.AddDate(0, 1, 2))
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("renewal tick: %v", err)
	}
	if _, err := relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay after renewal: %v", err)
	}
	if s := subStatus(t, deps, tenantID); s != "past_due" {
		t.Fatalf("status after failed charge = %s, want past_due", s)
	}
	failedAt := clk.Now()

	// Exhaust every retry (+1d, +3d, +7d all decline).
	for _, off := range []time.Duration{1 * day, 3 * day, 7 * day} {
		clk.Set(failedAt.Add(off))
		if err := runner.RunDue(ctx); err != nil {
			t.Fatalf("dunning tick at +%v: %v", off, err)
		}
		if _, err := relay.ProcessBatch(ctx); err != nil {
			t.Fatalf("relay at +%v: %v", off, err)
		}
	}
	exhaustedAt := failedAt.Add(7 * day)

	if s := subStatus(t, deps, tenantID); s != "grace" {
		t.Fatalf("status after dunning exhausted = %s, want grace", s)
	}
	if n := outboxCount(t, deps, tenantID, billingports.EventDunningExhausted); n != 1 {
		t.Fatalf("dunning_exhausted events = %d, want 1", n)
	}
	// The grace length is exactly the plan version's grace days.
	wantGraceEnd := exhaustedAt.AddDate(0, 0, graceDays)
	if ge := graceEndsAt(t, deps, tenantID); !ge.Equal(wantGraceEnd) {
		t.Fatalf("grace_ends_at = %v, want %v (grace length from pinned plan version)", ge, wantGraceEnd)
	}

	// Before the grace deadline: still in grace.
	clk.Set(wantGraceEnd.Add(-1 * day))
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("pre-deadline grace tick: %v", err)
	}
	if s := subStatus(t, deps, tenantID); s != "grace" {
		t.Fatalf("status before grace deadline = %s, want grace", s)
	}

	// Past the grace deadline: the grace scan suspends the subscription.
	clk.Set(wantGraceEnd.Add(1 * day))
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("post-deadline grace tick: %v", err)
	}
	if s := subStatus(t, deps, tenantID); s != "suspended" {
		t.Fatalf("status after grace elapsed = %s, want suspended", s)
	}
}

// prorationCatalog serves distinct plan-version prices per id so a plan change
// has a real price difference to prorate.
type prorationCatalog struct {
	prices map[uuid.UUID]int64
}

func (c *prorationCatalog) GetPlanVersion(_ context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error) {
	return catalogports.PlanVersionInfo{
		ID: id, PlanKey: "pro", Version: 2, Status: "published", Currency: "USD",
		Prices: []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: c.prices[id]}},
	}, nil
}

func (c *prorationCatalog) GetAddonVersion(_ context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error) {
	return catalogports.AddonVersionInfo{ID: id}, nil
}

// prorationSubs serves a live subscription with a fixed billing period so the
// proration has a period to scale against.
type prorationSubs struct {
	subID         uuid.UUID
	planVersionID uuid.UUID
	periodStart   time.Time
	periodEnd     time.Time
}

func (s *prorationSubs) GetLiveForTenant(_ context.Context, tenantID uuid.UUID) (subports.SubscriptionInfo, error) {
	return subports.SubscriptionInfo{
		ID: s.subID, TenantID: tenantID, PlanVersionID: s.planVersionID,
		BillingCycle: "monthly", Status: "active",
		CurrentPeriodStart: s.periodStart, CurrentPeriodEnd: s.periodEnd,
	}, nil
}

func (s *prorationSubs) GetAttachedAddons(_ context.Context, _ uuid.UUID) ([]subports.AddonAttachment, error) {
	return nil, nil
}

// TestPlanChangeTriggersProratedInvoiceLine proves the plan-changed consumer
// issues an invoice carrying a prorated line for the remaining-period price
// difference, computed with exact integer arithmetic.
func TestPlanChangeTriggersProratedInvoiceLine(t *testing.T) {
	clk := clock.NewFrozen(billingEpoch)
	deps := frozenBillingDeps(t, clk)

	oldPV, newPV := uuid.New(), uuid.New()
	tenantID, subID := uuid.New(), uuid.New()
	periodStart := billingEpoch
	periodEnd := billingEpoch.AddDate(0, 1, 0) // 31-day period (Jan)
	// The upgrade happens on day 15 of the period.
	clk.Set(billingEpoch.Add(15 * day))

	cat := &prorationCatalog{prices: map[uuid.UUID]int64{oldPV: 1000, newPV: 2000}}
	subs := &prorationSubs{subID: subID, planVersionID: newPV, periodStart: periodStart, periodEnd: periodEnd}
	mod := billing.New(deps, cat, subs, billing.WithPaymentProvider(fakeprovider.New()))

	ctx := context.Background()
	if err := planChangedHandler(t, mod)(ctx, planChangedEvent(tenantID, subID, oldPV, newPV)); err != nil {
		t.Fatalf("plan-changed handler: %v", err)
	}

	// (2000-1000) * remainingDays(16) / totalDays(31) = 516, truncated.
	const wantAmount = 516
	var (
		kind   string
		amount int64
	)
	if err := deps.Pool.QueryRow(ctx,
		`SELECT li.kind, li.amount_minor
		 FROM billing.invoice_line_items li
		 JOIN billing.invoices i ON i.id = li.invoice_id
		 WHERE i.tenant_id = $1 AND li.kind = 'proration'`, tenantID).Scan(&kind, &amount); err != nil {
		t.Fatalf("load proration line: %v", err)
	}
	if amount != wantAmount {
		t.Fatalf("proration line amount = %d, want %d (exact integer proration)", amount, wantAmount)
	}
}
