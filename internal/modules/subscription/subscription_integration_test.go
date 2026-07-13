//go:build integration

package subscription_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription"
	subpg "github.com/williamokano/entitlements/internal/modules/subscription/internal/adapters/postgres"
	subdomain "github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newDeps(t *testing.T) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	clk := clock.System
	return app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        events.NewBus(),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
}

// fakeCatalog returns a canned published plan version for any id. Prices can
// vary per version id (upgrade/downgrade tests); unlisted ids cost 1000/month.
type fakeCatalog struct {
	trialDays    int
	cardRequired bool
	amounts      map[uuid.UUID]int64
	addons       map[uuid.UUID]catalogports.AddonVersionInfo
}

func (f fakeCatalog) GetPlanVersion(_ context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error) {
	amount := int64(1000)
	if a, ok := f.amounts[id]; ok {
		amount = a
	}
	return catalogports.PlanVersionInfo{
		ID: id, PlanID: uuid.New(), PlanKey: "pro", Version: 1, Status: "published",
		Currency:     "USD",
		Prices:       []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: amount}},
		TrialEnabled: f.trialDays > 0, TrialDays: f.trialDays, CardRequired: f.cardRequired,
	}, nil
}

// GetAddonVersion returns a pro-compatible quantity-enabled addon unless the id
// is overridden.
func (f fakeCatalog) GetAddonVersion(_ context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error) {
	if av, ok := f.addons[id]; ok {
		return av, nil
	}
	return catalogports.AddonVersionInfo{
		ID: id, AddonID: uuid.New(), AddonKey: "extra-seats", Version: 1, Status: "published",
		Currency: "USD", QuantityAllowed: true, CompatiblePlanKeys: []string{"pro"},
	}, nil
}

// serverFor wraps a module handler with a resolved tenant + user principal.
func serverFor(h http.Handler, tenantID, userID uuid.UUID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithTenantID(r.Context(), tenantID)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: userID.String()})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func post(t *testing.T, url, body string) (int, string) { return do(t, http.MethodPost, url, body) }
func get(t *testing.T, url string) (int, string)        { return do(t, http.MethodGet, url, "") }

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestSubscriptionEndpointsCreateGetCancel(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{trialDays: 14})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	pv := uuid.New()
	status, body := post(t, srv.URL, `{"plan_version_id":"`+pv.String()+`","cycle":"monthly"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d (%s)", status, body)
	}
	if !strings.Contains(body, `"status":"trialing"`) {
		t.Fatalf("expected trialing (14-day trial), got %s", body)
	}

	// Get returns the live subscription.
	if status, gb := get(t, srv.URL); status != http.StatusOK || !strings.Contains(gb, `"status":"trialing"`) {
		t.Fatalf("get = %d %s", status, gb)
	}

	// Cancel at period end keeps it live.
	if status, cb := post(t, srv.URL+"/cancel", `{"immediate":false}`); status != http.StatusOK || !strings.Contains(cb, `"cancel_at_period_end":true`) {
		t.Fatalf("cancel-at-period-end = %d %s", status, cb)
	}
	if _, gb := get(t, srv.URL); strings.Contains(gb, `"status":"canceled"`) {
		t.Fatalf("subscription canceled immediately on at-period-end cancel: %s", gb)
	}

	// Immediate cancel terminates it.
	if status, _ := post(t, srv.URL+"/cancel", `{"immediate":true,"reason":"done"}`); status != http.StatusOK {
		t.Fatalf("cancel immediate status")
	}
	if status, _ := get(t, srv.URL); status != http.StatusNotFound {
		t.Fatalf("get after immediate cancel = %d, want 404 (no live subscription)", status)
	}
}

func TestSecondActiveSubscriptionForTenantRejected409(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("first create, want 201")
	}
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusConflict {
		t.Fatal("second create, want 409")
	}
}

func TestEveryTransitionRecordsHistoryAndOutboxEventAtomically(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	ctx := context.Background()

	// Create (active) then pause → two transitions, two outbox events.
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("create")
	}
	if status, _ := post(t, srv.URL+"/pause", ``); status != http.StatusOK {
		t.Fatal("pause")
	}

	var subID uuid.UUID
	if err := deps.Pool.QueryRow(ctx, `SELECT id FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&subID); err != nil {
		t.Fatalf("find subscription: %v", err)
	}
	var transitions, outboxEvents int
	if err := deps.Pool.QueryRow(ctx, `SELECT count(*) FROM subscription.subscription_transitions WHERE subscription_id = $1`, subID).Scan(&transitions); err != nil {
		t.Fatalf("count transitions: %v", err)
	}
	if err := deps.Pool.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'subscription.transitioned'`, tenantID).Scan(&outboxEvents); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if transitions != 2 || outboxEvents != 2 {
		t.Fatalf("transitions=%d outbox=%d, want 2 and 2", transitions, outboxEvents)
	}

	// Atomicity: a transaction that appends a transition then fails rolls back —
	// leaving neither the row nor any partial state.
	repo := subpg.New(deps.Pool)
	before := transitions
	_ = deps.UnitOfWork.Do(ctx, func(ctx context.Context) error {
		_ = repo.AppendTransition(ctx, &subdomain.Transition{
			ID: uuid.New(), SubscriptionID: subID, From: subdomain.StatePaused, To: subdomain.StateActive, Event: subdomain.EventResume, At: deps.Clock.Now(),
		})
		return errors.New("boom") // force rollback
	})
	var after int
	if err := deps.Pool.QueryRow(ctx, `SELECT count(*) FROM subscription.subscription_transitions WHERE subscription_id = $1`, subID).Scan(&after); err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if after != before {
		t.Fatalf("rollback left a transition row: before=%d after=%d", before, after)
	}
}

func TestScheduledChangePersistedAndVisibleViaREST(t *testing.T) {
	deps := newDeps(t)
	cheaper := uuid.New()
	mod := subscription.New(deps, fakeCatalog{amounts: map[uuid.UUID]int64{cheaper: 500}})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	original := uuid.New()
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+original.String()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("create")
	}

	// Cheaper plan → downgrade → scheduled, current pin unchanged.
	status, body := post(t, srv.URL+"/change-plan", `{"plan_version_id":"`+cheaper.String()+`","cycle":"monthly"}`)
	if status != http.StatusOK {
		t.Fatalf("change-plan = %d (%s)", status, body)
	}
	if !strings.Contains(body, `"plan_version_id":"`+original.String()+`"`) {
		t.Fatalf("downgrade re-pinned immediately: %s", body)
	}
	if !strings.Contains(body, `"scheduled_change"`) || !strings.Contains(body, cheaper.String()) {
		t.Fatalf("scheduled change not visible: %s", body)
	}

	// Survives a reload (persisted, not just in the response).
	if _, gb := get(t, srv.URL); !strings.Contains(gb, `"scheduled_change"`) {
		t.Fatalf("scheduled change not persisted: %s", gb)
	}

	// Cancelable.
	if status, cb := post(t, srv.URL+"/scheduled-change/cancel", ``); status != http.StatusOK || strings.Contains(cb, `"scheduled_change"`) {
		t.Fatalf("cancel scheduled change = %d %s", status, cb)
	}
	if _, gb := get(t, srv.URL); strings.Contains(gb, `"scheduled_change"`) {
		t.Fatalf("scheduled change survived cancel: %s", gb)
	}
	// Canceling again conflicts.
	if status, _ := post(t, srv.URL+"/scheduled-change/cancel", ``); status != http.StatusConflict {
		t.Fatal("second cancel, want 409")
	}
}

func TestApplyScheduledChangeAtBoundaryRepinsAndEmits(t *testing.T) {
	deps := newDeps(t)
	cheaper := uuid.New()
	mod := subscription.New(deps, fakeCatalog{amounts: map[uuid.UUID]int64{cheaper: 500}})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	ctx := context.Background()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("create")
	}
	if status, _ := post(t, srv.URL+"/change-plan", `{"plan_version_id":"`+cheaper.String()+`","cycle":"monthly"}`); status != http.StatusOK {
		t.Fatal("schedule downgrade")
	}

	var subID uuid.UUID
	if err := deps.Pool.QueryRow(ctx, `SELECT id FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&subID); err != nil {
		t.Fatalf("find subscription: %v", err)
	}

	// The hook T-021 will call at rollover.
	if err := mod.ApplyScheduledChange(ctx, subID); err != nil {
		t.Fatalf("ApplyScheduledChange: %v", err)
	}

	if _, gb := get(t, srv.URL); !strings.Contains(gb, `"plan_version_id":"`+cheaper.String()+`"`) || strings.Contains(gb, `"scheduled_change"`) {
		t.Fatalf("not re-pinned/cleared after apply: %s", gb)
	}
	var payload string
	if err := deps.Pool.QueryRow(ctx,
		`SELECT payload::text FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'subscription.plan_changed'`, tenantID).Scan(&payload); err != nil {
		t.Fatalf("plan_changed event: %v", err)
	}
	if !strings.Contains(payload, `"timing": "period_end"`) || !strings.Contains(payload, cheaper.String()) {
		t.Fatalf("payload = %s", payload)
	}

	// Idempotent: applying again with nothing scheduled is a no-op.
	if err := mod.ApplyScheduledChange(ctx, subID); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var count int
	if err := deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'subscription.plan_changed'`, tenantID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("plan_changed events = %d, want 1", count)
	}
}

func TestAddonAttachDetachEmitsSubscriptionAddonChanged(t *testing.T) {
	deps := newDeps(t)
	incompatible := uuid.New()
	mod := subscription.New(deps, fakeCatalog{addons: map[uuid.UUID]catalogports.AddonVersionInfo{
		incompatible: {ID: incompatible, Status: "published", QuantityAllowed: true, CompatiblePlanKeys: []string{"enterprise"}},
	}})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	ctx := context.Background()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("create")
	}

	// Incompatible addon rejected end-to-end.
	if status, _ := post(t, srv.URL+"/addons", `{"addon_version_id":"`+incompatible.String()+`","quantity":1}`); status != http.StatusBadRequest {
		t.Fatal("incompatible attach, want 400")
	}

	av := uuid.New()
	status, body := post(t, srv.URL+"/addons", `{"addon_version_id":"`+av.String()+`","quantity":3}`)
	if status != http.StatusOK || !strings.Contains(body, `"quantity":3`) {
		t.Fatalf("attach = %d %s", status, body)
	}

	// Quantity change on re-attach.
	if status, _ := post(t, srv.URL+"/addons", `{"addon_version_id":"`+av.String()+`","quantity":5}`); status != http.StatusOK {
		t.Fatal("quantity change")
	}

	// Detach.
	if status, db := do(t, http.MethodDelete, srv.URL+"/addons/"+av.String(), ""); status != http.StatusOK || strings.Contains(db, av.String()) {
		t.Fatalf("detach = %d %s", status, db)
	}
	// Detaching again is a 404.
	if status, _ := do(t, http.MethodDelete, srv.URL+"/addons/"+av.String(), ""); status != http.StatusNotFound {
		t.Fatal("second detach, want 404")
	}

	rows, err := deps.Pool.Query(ctx,
		`SELECT payload::text FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'subscription.addon_changed' ORDER BY occurred_at`, tenantID)
	if err != nil {
		t.Fatalf("query addon events: %v", err)
	}
	defer rows.Close()
	var payloads []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, p)
	}
	if len(payloads) != 3 {
		t.Fatalf("addon_changed events = %d, want 3 (attached, quantity_changed, detached)", len(payloads))
	}
	for i, want := range []string{`"action": "attached"`, `"action": "quantity_changed"`, `"action": "detached"`} {
		if !strings.Contains(payloads[i], want) {
			t.Fatalf("event %d = %s, want %s", i, payloads[i], want)
		}
	}
}

func TestGetAttachedAddonsReturnsAttachmentsForEntitlements(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	ctx := context.Background()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("create")
	}
	av := uuid.New()
	if status, _ := post(t, srv.URL+"/addons", `{"addon_version_id":"`+av.String()+`","quantity":4}`); status != http.StatusOK {
		t.Fatal("attach addon")
	}

	var subID uuid.UUID
	if err := deps.Pool.QueryRow(ctx, `SELECT id FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&subID); err != nil {
		t.Fatalf("find subscription: %v", err)
	}

	// The port the entitlements resolver reads: addon version id + quantity.
	attachments, err := mod.Port().GetAttachedAddons(ctx, subID)
	if err != nil {
		t.Fatalf("GetAttachedAddons: %v", err)
	}
	if len(attachments) != 1 || attachments[0].AddonVersionID != av || attachments[0].Quantity != 4 {
		t.Fatalf("attachments = %+v, want one {%s, 4}", attachments, av)
	}
}

func TestUnauthenticatedSubscriptionRouteRejected(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	// Tenant set but no principal.
	tenantID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mod.Handler().ServeHTTP(w, r.WithContext(authctx.WithTenantID(r.Context(), tenantID)))
	}))
	defer srv.Close()
	if status, _ := get(t, srv.URL); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated get = %d, want 401", status)
	}
}
