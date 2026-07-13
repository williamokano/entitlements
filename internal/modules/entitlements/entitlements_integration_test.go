//go:build integration

package entitlements_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/app"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/entitlements"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/service"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
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

// fakeCatalog serves a plan version with configurable feature grants and addon
// versions with configurable deltas.
type fakeCatalog struct {
	grants map[uuid.UUID]map[string]any
	addons map[uuid.UUID]catalogports.AddonVersionInfo
}

func (f fakeCatalog) GetPlanVersion(_ context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error) {
	return catalogports.PlanVersionInfo{
		ID: id, PlanID: uuid.New(), PlanKey: "pro", Version: 1, Status: "published",
		Currency: "USD", FeatureGrants: f.grants[id],
	}, nil
}

func (f fakeCatalog) GetAddonVersion(_ context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error) {
	return f.addons[id], nil
}

// fakeSubs is a programmable SubscriptionReader.
type fakeSubs struct {
	sub    subports.SubscriptionInfo
	hasSub bool
	addons []subports.AddonAttachment
}

func (f *fakeSubs) GetLiveForTenant(_ context.Context, tenantID uuid.UUID) (subports.SubscriptionInfo, error) {
	if !f.hasSub {
		return subports.SubscriptionInfo{}, apperrNotFound()
	}
	s := f.sub
	s.TenantID = tenantID
	return s, nil
}

func (f *fakeSubs) GetAttachedAddons(_ context.Context, _ uuid.UUID) ([]subports.AddonAttachment, error) {
	return f.addons, nil
}

func apperrNotFound() error {
	// Mirror the subscription repo's NotFound so Resolve's KindNotFound branch
	// treats "no live subscription" as defaults-only.
	return apperr.NotFound("subscription not found")
}

// modFor wires an entitlements module with the given catalog + subs fakes and
// the given unknown-feature policy.
func modFor(deps app.Deps, cat service.CatalogReader, subs service.SubscriptionReader, policy string) *entitlements.Module {
	deps.Config.EntitlementsUnknownFeaturePolicy = policy
	return entitlements.New(deps, cat, subs)
}

func serverFor(h http.Handler, tenantID, userID uuid.UUID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithTenantID(r.Context(), tenantID)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: userID.String()})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// seedFeature inserts a registry feature directly (via the service through the
// module) so tests can compose scenarios.
func seedFeature(t *testing.T, mod *entitlements.Module, in service.FeatureInput) {
	t.Helper()
	if _, err := mod.Service().CreateFeature(context.Background(), in); err != nil {
		t.Fatalf("seed feature %s: %v", in.Key, err)
	}
}

func TestFeatureRegistryCRUDNewFeatureNeedsNoDeploy(t *testing.T) {
	deps := newDeps(t)
	subs := &fakeSubs{hasSub: false}
	mod := modFor(deps, fakeCatalog{}, subs, "deny")
	ctx := context.Background()
	tenantID := uuid.New()

	// A brand-new feature appears in resolution with its default immediately —
	// no deploy, just an insert.
	seedFeature(t, mod, service.FeatureInput{Key: "api_access", Type: "boolean", DefaultValue: false})

	set, err := mod.Service().GetAll(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	e, ok := set["api_access"]
	if !ok {
		t.Fatalf("new feature not in resolved set: %+v", set)
	}
	if e.Value != false || e.Source != "default" {
		t.Fatalf("api_access = %#v/%s, want false/default", e.Value, e.Source)
	}

	// CRUD: get, list, update (raise default), archive.
	if _, err := mod.Service().UpdateFeature(ctx, "api_access", service.FeatureInput{Key: "api_access", Type: "boolean", DefaultValue: true}); err != nil {
		t.Fatalf("update: %v", err)
	}
	set, _ = mod.Service().GetAll(ctx, tenantID)
	if set["api_access"].Value != true {
		t.Fatalf("updated default not reflected: %#v", set["api_access"].Value)
	}
	features, err := mod.Service().ListFeatures(ctx)
	if err != nil || len(features) != 1 {
		t.Fatalf("list = %v (%d), want 1", err, len(features))
	}
}

func TestArchivedFeatureStopsGrantingButKeepsHistory(t *testing.T) {
	deps := newDeps(t)
	subs := &fakeSubs{hasSub: false}
	mod := modFor(deps, fakeCatalog{}, subs, "deny")
	ctx := context.Background()
	tenantID := uuid.New()

	seedFeature(t, mod, service.FeatureInput{Key: "reports", Type: "boolean", DefaultValue: true})

	set, _ := mod.Service().GetAll(ctx, tenantID)
	if _, ok := set["reports"]; !ok {
		t.Fatal("feature should grant before archive")
	}

	if _, err := mod.Service().ArchiveFeature(ctx, "reports"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// Stops granting: no longer in the resolved set.
	set, _ = mod.Service().GetAll(ctx, tenantID)
	if _, ok := set["reports"]; ok {
		t.Fatal("archived feature still granting")
	}

	// Keeps history: the row survives, marked inactive.
	var active bool
	if err := deps.Pool.QueryRow(ctx, `SELECT active FROM entitlements.features WHERE key = 'reports'`).Scan(&active); err != nil {
		t.Fatalf("feature row gone: %v", err)
	}
	if active {
		t.Fatal("archived feature should be inactive, not deleted")
	}
}

func TestMaterializedSetRebuiltOnSubscriptionPlanChanged(t *testing.T) {
	deps := newDeps(t)
	pvA, pvB := uuid.New(), uuid.New()
	cat := fakeCatalog{grants: map[uuid.UUID]map[string]any{
		pvA: {"seats": float64(10)},
		pvB: {"seats": float64(25)},
	}}
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pvA}}
	mod := modFor(deps, cat, subs, "deny")
	tenantID := uuid.New()

	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	// Deliver a plan_changed event through the module's subscription handler.
	deliverPlanChanged(t, mod, tenantID)
	assertEffectiveSeats(t, deps.Pool, tenantID, 10)

	// Re-pin to plan B and re-deliver: the materialized set is rebuilt.
	subs.sub.PlanVersionID = pvB
	deliverPlanChanged(t, mod, tenantID)
	assertEffectiveSeats(t, deps.Pool, tenantID, 25)
}

func TestMaterializationIdempotentUnderDuplicateEvents(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	cat := fakeCatalog{grants: map[uuid.UUID]map[string]any{pv: {"seats": float64(10)}}}
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, cat, subs, "deny")
	ctx := context.Background()
	tenantID := uuid.New()
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	// Same event id delivered twice: the Idempotent guard drops the duplicate and
	// exactly one summary event is emitted.
	evt := planChangedEvent(tenantID)
	handler := planChangedHandler(mod)
	if err := handler(ctx, evt); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := handler(ctx, evt); err != nil {
		t.Fatalf("duplicate delivery: %v", err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 10)

	var summaryEvents int
	if err := deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'entitlements.summary_changed'`, tenantID).Scan(&summaryEvents); err != nil {
		t.Fatal(err)
	}
	if summaryEvents != 1 {
		t.Fatalf("summary events = %d, want 1", summaryEvents)
	}
}

func TestEntitlementsSummaryChangedEmittedOnEffectiveSetChange(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	cat := fakeCatalog{grants: map[uuid.UUID]map[string]any{pv: {"seats": float64(10)}}}
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, cat, subs, "deny")
	ctx := context.Background()
	tenantID := uuid.New()
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})
	seedFeature(t, mod, service.FeatureInput{Key: "sso", Type: "boolean", DefaultValue: false})

	// First materialization changes the set (default → plan) → one event carrying
	// the full set.
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	payloads := summaryPayloads(t, deps.Pool, tenantID)
	if len(payloads) != 1 {
		t.Fatalf("summary events = %d, want 1", len(payloads))
	}
	var got struct {
		Entitlements map[string]struct {
			Value  any    `json:"value"`
			Source string `json:"source"`
		} `json:"entitlements"`
	}
	if err := json.Unmarshal([]byte(payloads[0]), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	// Full set: both features present.
	if _, ok := got.Entitlements["seats"]; !ok {
		t.Errorf("payload missing seats: %s", payloads[0])
	}
	if _, ok := got.Entitlements["sso"]; !ok {
		t.Errorf("payload missing sso (full set expected): %s", payloads[0])
	}
	if got.Entitlements["seats"].Source != "plan" {
		t.Errorf("seats source = %s, want plan", got.Entitlements["seats"].Source)
	}

	// Re-materializing with no change emits nothing new.
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatalf("re-materialize: %v", err)
	}
	if again := summaryPayloads(t, deps.Pool, tenantID); len(again) != 1 {
		t.Fatalf("no-op re-materialize emitted an event: %d total", len(again))
	}
}

func TestGetAllEntitlementsSingleCall(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	cat := fakeCatalog{grants: map[uuid.UUID]map[string]any{pv: {"seats": float64(10), "sso": true}}}
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv}}
	mod := modFor(deps, cat, subs, "deny")
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})
	seedFeature(t, mod, service.FeatureInput{Key: "sso", Type: "boolean", DefaultValue: false})
	seedFeature(t, mod, service.FeatureInput{Key: "theme", Type: "config", DefaultValue: "light"})

	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	status, body := get(t, srv.URL)
	if status != http.StatusOK {
		t.Fatalf("GET /entitlements = %d (%s)", status, body)
	}
	// The whole set comes back in one call.
	for _, key := range []string{"seats", "sso", "theme"} {
		if !strings.Contains(body, `"`+key+`"`) {
			t.Errorf("response missing %s: %s", key, body)
		}
	}
	if !strings.Contains(body, `"source":"plan"`) || !strings.Contains(body, `"source":"default"`) {
		t.Errorf("expected plan and default sources: %s", body)
	}
}

func TestGetSingleEntitlement(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	av := uuid.New()
	cat := fakeCatalog{
		grants: map[uuid.UUID]map[string]any{pv: {"seats": float64(10)}},
		addons: map[uuid.UUID]catalogports.AddonVersionInfo{
			av: {ID: av, Status: "published", Deltas: []catalogports.DeltaInfo{{FeatureKey: "seats", Kind: "limit_delta", Amount: 10}}},
		},
	}
	subs := &fakeSubs{
		hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv},
		addons: []subports.AddonAttachment{{AddonVersionID: av, Quantity: 1}},
	}
	mod := modFor(deps, cat, subs, "deny")
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	// plan 10 + addon 10 (x1) => 20 from the single-key endpoint.
	status, body := get(t, srv.URL+"/seats")
	if status != http.StatusOK {
		t.Fatalf("GET /entitlements/seats = %d (%s)", status, body)
	}
	if !strings.Contains(body, `"value":20`) || !strings.Contains(body, `"source":"addon"`) {
		t.Fatalf("seats = %s, want value 20 / source addon", body)
	}

	// Unknown feature key → 404 under deny policy.
	if status, _ := get(t, srv.URL+"/nonexistent"); status != http.StatusNotFound {
		t.Fatalf("GET unknown key = %d, want 404", status)
	}
}

func TestAddonLimitDeltaTimesQuantityMaterialized(t *testing.T) {
	deps := newDeps(t)
	pv := uuid.New()
	av := uuid.New()
	cat := fakeCatalog{
		grants: map[uuid.UUID]map[string]any{pv: {"seats": float64(10)}},
		addons: map[uuid.UUID]catalogports.AddonVersionInfo{
			av: {ID: av, Status: "published", Deltas: []catalogports.DeltaInfo{{FeatureKey: "seats", Kind: "limit_delta", Amount: 10}}},
		},
	}
	subs := &fakeSubs{
		hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pv},
		addons: []subports.AddonAttachment{{AddonVersionID: av, Quantity: 1}},
	}
	mod := modFor(deps, cat, subs, "deny")
	ctx := context.Background()
	tenantID := uuid.New()
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(0)})

	// x1 => 10 + 10 = 20.
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 20)

	// x2 => 10 + 10*2 = 30.
	subs.addons[0].Quantity = 2
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	assertEffectiveSeats(t, deps.Pool, tenantID, 30)
}

// --- helpers ---

func planChangedEvent(tenantID uuid.UUID) events.Event {
	payload, _ := json.Marshal(subports.SubscriptionPlanChanged{TenantID: tenantID})
	return events.Event{ID: uuid.New(), OccurredAt: time.Now().UTC(), TenantID: tenantID, Module: "subscription", Type: subports.EventSubscriptionPlanChanged, Payload: payload}
}

func planChangedHandler(mod *entitlements.Module) events.Handler {
	for _, s := range mod.Subscriptions() {
		if s.EventType == subports.EventSubscriptionPlanChanged {
			return s.Handler
		}
	}
	panic("no plan_changed subscription")
}

func deliverPlanChanged(t *testing.T, mod *entitlements.Module, tenantID uuid.UUID) {
	t.Helper()
	if err := planChangedHandler(mod)(context.Background(), planChangedEvent(tenantID)); err != nil {
		t.Fatalf("deliver plan_changed: %v", err)
	}
}

func assertEffectiveSeats(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, want int64) {
	t.Helper()
	var valRaw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT value FROM entitlements.effective_entitlements WHERE tenant_id = $1 AND feature_key = 'seats'`, tenantID).Scan(&valRaw); err != nil {
		t.Fatalf("read effective seats: %v", err)
	}
	var got int64
	if err := json.Unmarshal(valRaw, &got); err != nil {
		t.Fatalf("unmarshal seats: %v", err)
	}
	if got != want {
		t.Fatalf("effective seats = %d, want %d", got, want)
	}
}

func summaryPayloads(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT payload::text FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'entitlements.summary_changed' ORDER BY occurred_at`, tenantID)
	if err != nil {
		t.Fatalf("query summary events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}
