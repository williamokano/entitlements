//go:build integration

package entitlements_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/service"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/audit"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

// frozenDeps builds module deps backed by the given (frozen) clock, so
// override-expiry tests can drive time deterministically.
func frozenDeps(t *testing.T, clk clock.Clock) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	return app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        events.NewBus(),
		Audit:      audit.NewWriter(pool, ids, clk),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
}

// actorCtx returns a context carrying the tenant and an authenticated principal,
// as the middleware would set for an admin call.
func actorCtx(tenantID uuid.UUID, actor string) context.Context {
	ctx := authctx.WithTenantID(context.Background(), tenantID)
	return authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: actor})
}

func planSeats(pv uuid.UUID, seats int) fakeCatalog {
	return fakeCatalog{grants: map[uuid.UUID]map[string]any{pv: {"seats": float64(seats)}}}
}

// TestOverrideBoostsEffectiveValueImmediately: an override wins over the plan
// value the moment it is created — no event round-trip needed.
func TestOverrideBoostsEffectiveValueImmediately(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, planSeats(pv, 10), subs, "deny")
	tenantID, userID := uuid.New(), uuid.New()
	ctx := actorCtx(tenantID, userID.String())
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	// Baseline: plan grants 10.
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatalf("baseline materialize: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 10)

	// Create an override to 50: it re-materializes in the same call.
	view, err := mod.Service().CreateOverride(ctx, service.OverrideInput{FeatureKey: "seats", Value: int64(50), Reason: "enterprise trial"})
	if err != nil {
		t.Fatalf("create override: %v", err)
	}
	if view.Actor != userID.String() {
		t.Fatalf("override actor = %q, want %q", view.Actor, userID.String())
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 50)

	// And the live read reports the override as the winning source.
	e, err := mod.Service().Get(ctx, tenantID, "seats")
	if err != nil {
		t.Fatalf("get seats: %v", err)
	}
	if e.Source != "override" {
		t.Fatalf("seats source = %s, want override", e.Source)
	}
}

// TestOverrideDeleteRestoresPlanAddonValue: removing an override reverts the
// effective value to plan + addons.
func TestOverrideDeleteRestoresPlanAddonValue(t *testing.T) {
	deps := newDeps(t)
	pv, av := uuid.New(), uuid.New()
	cat := planSeats(pv, 10)
	cat.addons = map[uuid.UUID]catalogports.AddonVersionInfo{
		av: {ID: av, Status: "published", Deltas: []catalogports.DeltaInfo{{FeatureKey: "seats", Kind: "limit_delta", Amount: 10}}},
	}
	subs := &fakeSubs{
		hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv},
		addons: []subports.AddonAttachment{{AddonVersionID: av, Quantity: 1}},
	}
	mod := modFor(deps, cat, subs, "deny")
	tenantID, userID := uuid.New(), uuid.New()
	ctx := actorCtx(tenantID, userID.String())
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	// plan 10 + addon 10 = 20.
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 20)

	// Override to 99, then delete it: value returns to 20 (plan + addon).
	view, err := mod.Service().CreateOverride(ctx, service.OverrideInput{FeatureKey: "seats", Value: int64(99), Reason: "temporary bump"})
	if err != nil {
		t.Fatalf("create override: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 99)

	if err := mod.Service().DeleteOverride(ctx, view.ID); err != nil {
		t.Fatalf("delete override: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 20)

	e, err := mod.Service().Get(ctx, tenantID, "seats")
	if err != nil {
		t.Fatalf("get seats: %v", err)
	}
	if e.Source != "addon" {
		t.Fatalf("seats source after delete = %s, want addon", e.Source)
	}
}

// TestOverrideExpiryJobRevertsValue: the recurring expiry job, driven by a
// frozen clock, reverts and removes an override once its expires_at has passed.
func TestOverrideExpiryJobRevertsValue(t *testing.T) {
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	deps := frozenDeps(t, clk)
	pv := uuid.New()
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, planSeats(pv, 10), subs, "deny")
	tenantID, userID := uuid.New(), uuid.New()
	ctx := actorCtx(tenantID, userID.String())
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	// A time-bound override to 50, expiring in 30 minutes.
	expiresAt := clk.Now().Add(30 * time.Minute)
	if _, err := mod.Service().CreateOverride(ctx, service.OverrideInput{
		FeatureKey: "seats", Value: int64(50), Reason: "72h trial boost", ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatalf("create override: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 50)

	// Register the expiry job on a runner driven by the same frozen clock.
	runner := jobs.NewRunner(deps.Pool, id.UUIDv7{}, clk, slog.New(slog.NewJSONHandler(io.Discard, nil)), time.Millisecond)
	if err := mod.RegisterJobs(runner); err != nil {
		t.Fatalf("register jobs: %v", err)
	}

	// Before expiry, the job is a no-op.
	if err := runner.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue before expiry: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 50)

	// Advance past the expiry and past the job interval, then run the job: the
	// effective value reverts to the plan value and the override row is gone.
	clk.Advance(time.Hour)
	if err := runner.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue after expiry: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 10)

	var remaining int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM entitlements.tenant_overrides WHERE tenant_id = $1`, tenantID).Scan(&remaining); err != nil {
		t.Fatalf("count overrides: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expired override rows remaining = %d, want 0", remaining)
	}
}

// TestOverrideChangesFullyAudited: create, update, and delete each write a
// platform.audit_log entry carrying the reason and actor.
func TestOverrideChangesFullyAudited(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, planSeats(pv, 10), subs, "deny")
	tenantID, userID := uuid.New(), uuid.New()
	ctx := actorCtx(tenantID, userID.String())
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	view, err := mod.Service().CreateOverride(ctx, service.OverrideInput{FeatureKey: "seats", Value: int64(50), Reason: "create reason"})
	if err != nil {
		t.Fatalf("create override: %v", err)
	}
	if _, err := mod.Service().UpdateOverride(ctx, view.ID, service.OverrideInput{FeatureKey: "seats", Value: int64(75), Reason: "update reason"}); err != nil {
		t.Fatalf("update override: %v", err)
	}
	if err := mod.Service().DeleteOverride(ctx, view.ID); err != nil {
		t.Fatalf("delete override: %v", err)
	}

	for _, tc := range []struct {
		action string
		reason string
	}{
		{"entitlement.override.created", "create reason"},
		{"entitlement.override.updated", "update reason"},
		{"entitlement.override.deleted", "update reason"}, // delete carries the override's last reason
	} {
		var (
			actor, reason string
			gotTenant     uuid.UUID
		)
		err := deps.Pool.QueryRow(context.Background(),
			`SELECT actor, tenant_id, reason FROM platform.audit_log WHERE action = $1 AND tenant_id = $2 ORDER BY created_at DESC LIMIT 1`,
			tc.action, tenantID).Scan(&actor, &gotTenant, &reason)
		if err != nil {
			t.Fatalf("audit row for %s: %v", tc.action, err)
		}
		if actor != userID.String() {
			t.Errorf("%s actor = %q, want %q", tc.action, actor, userID.String())
		}
		if reason == "" {
			t.Errorf("%s reason is empty, want a reason", tc.action)
		}
	}
}

// TestTimeBoundOverrideVisibleInEntitlementsResponse: the GET /entitlements
// response surfaces expires_at and the override source for a time-bound override.
func TestTimeBoundOverrideVisibleInEntitlementsResponse(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, planSeats(pv, 10), subs, "deny")
	tenantID, userID := uuid.New(), uuid.New()
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	// Create the override over HTTP, with a future expiry.
	expiresAt := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	body, _ := json.Marshal(map[string]any{
		"feature_key": "seats",
		"value":       50,
		"reason":      "enterprise negotiation",
		"expires_at":  expiresAt.Format(time.RFC3339),
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/overrides", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST override: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("POST override = %d (%s)", resp.StatusCode, b)
	}
	_ = resp.Body.Close()

	// GET /entitlements surfaces value 50, source override, and expires_at.
	status, respBody := get(t, srv.URL)
	if status != http.StatusOK {
		t.Fatalf("GET /entitlements = %d (%s)", status, respBody)
	}
	var parsed struct {
		Entitlements map[string]struct {
			Value     any     `json:"value"`
			Source    string  `json:"source"`
			ExpiresAt *string `json:"expires_at"`
		} `json:"entitlements"`
	}
	if err := json.Unmarshal([]byte(respBody), &parsed); err != nil {
		t.Fatalf("unmarshal response: %v (%s)", err, respBody)
	}
	seats, ok := parsed.Entitlements["seats"]
	if !ok {
		t.Fatalf("seats absent from response: %s", respBody)
	}
	if seats.Source != "override" {
		t.Errorf("seats source = %s, want override", seats.Source)
	}
	if seats.ExpiresAt == nil {
		t.Fatalf("seats expires_at missing from response: %s", respBody)
	}
	if got, err := time.Parse(time.RFC3339, *seats.ExpiresAt); err != nil || !got.Equal(expiresAt) {
		t.Errorf("seats expires_at = %v (err %v), want %v", got, err, expiresAt)
	}
}
