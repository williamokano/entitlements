//go:build integration

package subscription_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/subscription"
	"github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// frozenDeps builds module deps around a caller-controlled frozen clock so
// renewal/trial timing is deterministic.
func frozenDeps(t *testing.T, clk clock.Clock, billingDisabled bool) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	return app.Deps{
		Config:     config.Config{BillingDisabled: billingDisabled, TrialEndingDays: 3},
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        events.NewBus(),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
}

func newRunner(deps app.Deps, clk clock.Clock) *jobs.Runner {
	return jobs.NewRunner(deps.Pool, deps.IDs, clk, deps.Logger, time.Second)
}

func countEvents(t *testing.T, deps app.Deps, tenantID uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = $2`, tenantID, eventType).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", eventType, err)
	}
	return n
}

func subFields(t *testing.T, deps app.Deps, tenantID uuid.UUID) (id uuid.UUID, planVersionID uuid.UUID, periodEnd time.Time, status string) {
	t.Helper()
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT id, plan_version_id, current_period_end, status FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).
		Scan(&id, &planVersionID, &periodEnd, &status); err != nil {
		t.Fatalf("load subscription: %v", err)
	}
	return
}

func TestRenewalDueEmittedExactlyOncePerPeriod(t *testing.T) {
	clk := clock.NewFrozen(epoch)
	deps := frozenDeps(t, clk, false) // billing enabled: no auto-advance, marker must gate
	mod := subscription.New(deps, fakeCatalog{})
	runner := newRunner(deps, clk)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatal(err)
	}
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 {
		t.Fatal("create")
	}

	ctx := context.Background()
	clk.Set(epoch.AddDate(0, 1, 2)) // past the period end
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	clk.Advance(2 * time.Minute) // let the job interval elapse so it runs again
	if err := runner.RunDue(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if n := countEvents(t, deps, tenantID, ports.EventSubscriptionRenewalDue); n != 1 {
		t.Fatalf("renewal_due events = %d, want exactly 1", n)
	}
}

func TestTwoRunnersOneRenewalEmission(t *testing.T) {
	clk := clock.NewFrozen(epoch)
	deps := frozenDeps(t, clk, false)
	mod := subscription.New(deps, fakeCatalog{})
	r1, r2 := newRunner(deps, clk), newRunner(deps, clk)
	if err := mod.RegisterJobs(r1); err != nil {
		t.Fatal(err)
	}
	if err := mod.RegisterJobs(r2); err != nil {
		t.Fatal(err)
	}
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 {
		t.Fatal("create")
	}

	clk.Set(epoch.AddDate(0, 1, 2))
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, r := range []*jobs.Runner{r1, r2} {
		wg.Add(1)
		go func(r *jobs.Runner) { defer wg.Done(); _ = r.RunDue(ctx) }(r)
	}
	wg.Wait()

	if n := countEvents(t, deps, tenantID, ports.EventSubscriptionRenewalDue); n != 1 {
		t.Fatalf("renewal_due events = %d, want exactly 1 (advisory lock elects one runner)", n)
	}
}

func TestScheduledDowngradeAppliedAtRollover(t *testing.T) {
	clk := clock.NewFrozen(epoch)
	deps := frozenDeps(t, clk, true) // billing disabled: rollover auto-advances + applies the change
	cheaper := uuid.New()
	mod := subscription.New(deps, fakeCatalog{amounts: map[uuid.UUID]int64{cheaper: 500}})
	runner := newRunner(deps, clk)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatal(err)
	}
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 {
		t.Fatal("create")
	}
	if status, _ := post(t, srv.URL+"/change-plan", `{"plan_version_id":"`+cheaper.String()+`","cycle":"monthly"}`); status != 200 {
		t.Fatal("schedule downgrade")
	}

	clk.Set(epoch.AddDate(0, 1, 2))
	if err := runner.RunDue(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	_, pv, _, _ := subFields(t, deps, tenantID)
	if pv != cheaper {
		t.Fatalf("plan not re-pinned at rollover: pv=%s want %s", pv, cheaper)
	}
	var payload string
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT payload::text FROM platform.outbox WHERE tenant_id = $1 AND event_type = $2`, tenantID, ports.EventSubscriptionPlanChanged).Scan(&payload); err != nil {
		t.Fatalf("plan_changed event: %v", err)
	}
	if want := `"timing": "period_end"`; !strings.Contains(payload, want) {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}

func TestBillingDisabledAutoAdvancesPeriod(t *testing.T) {
	clk := clock.NewFrozen(epoch)
	deps := frozenDeps(t, clk, true)
	mod := subscription.New(deps, fakeCatalog{})
	runner := newRunner(deps, clk)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatal(err)
	}
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 {
		t.Fatal("create")
	}

	_, _, endBefore, _ := subFields(t, deps, tenantID)
	clk.Set(epoch.AddDate(0, 1, 2))
	if err := runner.RunDue(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	_, _, endAfter, _ := subFields(t, deps, tenantID)
	if !endAfter.After(endBefore) {
		t.Fatalf("period not advanced: before=%v after=%v", endBefore, endAfter)
	}
}

func TestPeriodNotAdvancedUntilInvoicePaidWhenBillingEnabled(t *testing.T) {
	clk := clock.NewFrozen(epoch)
	deps := frozenDeps(t, clk, false) // billing enabled
	mod := subscription.New(deps, fakeCatalog{})
	runner := newRunner(deps, clk)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatal(err)
	}
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 {
		t.Fatal("create")
	}

	_, _, endBefore, _ := subFields(t, deps, tenantID)
	clk.Set(epoch.AddDate(0, 1, 2))
	if err := runner.RunDue(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	subID, _, endAfterTick, _ := subFields(t, deps, tenantID)
	if !endAfterTick.Equal(endBefore) {
		t.Fatalf("period advanced without payment: before=%v after=%v", endBefore, endAfterTick)
	}
	if n := countEvents(t, deps, tenantID, ports.EventSubscriptionRenewalDue); n != 1 {
		t.Fatalf("renewal_due = %d, want 1", n)
	}

	// Invoice paid → the consumer advances the period.
	invoicePaid(t, mod, tenantID, subID)
	_, _, endAfterPaid, _ := subFields(t, deps, tenantID)
	if !endAfterPaid.After(endBefore) {
		t.Fatalf("period not advanced after invoice paid: before=%v after=%v", endBefore, endAfterPaid)
	}
}

func TestTrialEndingEmittedConfiguredDaysBefore(t *testing.T) {
	clk := clock.NewFrozen(epoch)
	deps := frozenDeps(t, clk, true)
	mod := subscription.New(deps, fakeCatalog{trialDays: 14})
	runner := newRunner(deps, clk)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatal(err)
	}
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	if status, body := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 || !strings.Contains(body, `"status":"trialing"`) {
		t.Fatalf("create trial = %d %s", status, body)
	}

	ctx := context.Background()
	// Trial ends at epoch+14d; the 3-day window opens at epoch+11d.
	clk.Set(epoch.AddDate(0, 0, 10)) // before the window
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, deps, tenantID, ports.EventSubscriptionTrialEnding); n != 0 {
		t.Fatalf("trial_ending emitted before the window: %d", n)
	}
	clk.Set(epoch.AddDate(0, 0, 12)) // inside the window
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Minute)
	if err := runner.RunDue(ctx); err != nil { // second tick must not re-emit
		t.Fatal(err)
	}
	if n := countEvents(t, deps, tenantID, ports.EventSubscriptionTrialEnding); n != 1 {
		t.Fatalf("trial_ending events = %d, want exactly 1", n)
	}
}

func TestTrialEndedConvertsToActiveOrExpiresPerPlanConfig(t *testing.T) {
	run := func(t *testing.T, cardRequired bool, wantStatus, wantOutcome string) {
		clk := clock.NewFrozen(epoch)
		deps := frozenDeps(t, clk, true)
		mod := subscription.New(deps, fakeCatalog{trialDays: 14, cardRequired: cardRequired})
		runner := newRunner(deps, clk)
		if err := mod.RegisterJobs(runner); err != nil {
			t.Fatal(err)
		}
		tenantID, userID := uuid.New(), uuid.New()
		srv := serverFor(mod.Handler(), tenantID, userID)
		defer srv.Close()
		if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != 201 {
			t.Fatal("create trial")
		}

		clk.Set(epoch.AddDate(0, 0, 14).Add(time.Hour)) // past trial end
		if err := runner.RunDue(context.Background()); err != nil {
			t.Fatalf("tick: %v", err)
		}
		_, _, _, status := subFields(t, deps, tenantID)
		if status != wantStatus {
			t.Fatalf("status = %s, want %s", status, wantStatus)
		}
		var payload string
		if err := deps.Pool.QueryRow(context.Background(),
			`SELECT payload::text FROM platform.outbox WHERE tenant_id = $1 AND event_type = $2`, tenantID, ports.EventSubscriptionTrialEnded).Scan(&payload); err != nil {
			t.Fatalf("trial_ended event: %v", err)
		}
		if !strings.Contains(payload, `"outcome": "`+wantOutcome+`"`) {
			t.Fatalf("payload = %s, want outcome %s", payload, wantOutcome)
		}
	}
	t.Run("no card converts to active", func(t *testing.T) { run(t, false, "active", ports.TrialConverted) })
	t.Run("card required expires", func(t *testing.T) { run(t, true, "expired", ports.TrialExpired) })
}

// invoicePaid drives the module's InvoicePaid consumer directly, as the relay
// would on a billing.invoice_paid event.
func invoicePaid(t *testing.T, mod *subscription.Module, tenantID, subID uuid.UUID) {
	t.Helper()
	payload, _ := json.Marshal(ports.BillingInvoicePaid{SubscriptionID: subID})
	subs := mod.Subscriptions()
	if len(subs) == 0 {
		t.Fatal("module exposes no subscriptions")
	}
	e := events.Event{ID: uuid.New(), TenantID: tenantID, Module: "billing", Type: ports.EventBillingInvoicePaid, Payload: payload}
	if err := subs[0].Handler(context.Background(), e); err != nil {
		t.Fatalf("invoice-paid handler: %v", err)
	}
}
