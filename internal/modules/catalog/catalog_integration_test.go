//go:build integration

package catalog_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/catalog"
	"github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newDeps(t *testing.T) (app.Deps, *events.Bus) {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	clk := clock.System
	bus := events.NewBus()
	return app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        bus,
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}, bus
}

// adminServer wraps the handler with an authenticated principal (admin routes
// require one).
func adminServer(h http.Handler) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{Kind: authctx.PrincipalUser, Subject: uuid.NewString()})
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

// createPublishedPlan creates a plan, sets v1 content, publishes v1, and makes
// the plan public. Returns plan id and v1 id.
func createPublishedPlan(t *testing.T, url, key string, monthly int64) (planID, v1ID uuid.UUID) {
	t.Helper()
	status, body := post(t, url+"/plans", `{"key":"`+key+`","name":"`+key+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create plan: %d (%s)", status, body)
	}
	var created struct {
		Plan    struct{ ID uuid.UUID } `json:"plan"`
		Version struct{ ID uuid.UUID } `json:"version"`
	}
	mustJSON(t, body, &created)
	planID, v1ID = created.Plan.ID, created.Version.ID

	content := `{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":` + strconv.FormatInt(monthly, 10) + `}],"grace_days":7,"feature_grants":{"max_projects":10}}`
	if status, b := do(t, http.MethodPatch, url+"/plans/"+planID.String()+"/versions/"+v1ID.String(), content); status != http.StatusOK {
		t.Fatalf("update v1: %d (%s)", status, b)
	}
	if status, b := post(t, url+"/plans/"+planID.String()+"/versions/"+v1ID.String()+"/publish", ""); status != http.StatusOK {
		t.Fatalf("publish v1: %d (%s)", status, b)
	}
	if status, _ := post(t, url+"/plans/"+planID.String()+"/public", `{"public":true}`); status != http.StatusOK {
		t.Fatalf("set public")
	}
	return planID, v1ID
}

func TestPlanCRUDAndPublishCreatesNewImmutableVersion(t *testing.T) {
	deps, _ := newDeps(t)
	mod := catalog.New(deps)
	srv := adminServer(mod.Handler())
	defer srv.Close()

	planID, v1ID := createPublishedPlan(t, srv.URL, "pro", 1000)

	// Editing the published v1 is rejected (immutable).
	if status, _ := do(t, http.MethodPatch, srv.URL+"/plans/"+planID.String()+"/versions/"+v1ID.String(),
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":9999}]}`); status != http.StatusConflict {
		t.Fatalf("edit published version, want 409")
	}

	// Open v2, change the price, publish it.
	status, body := post(t, srv.URL+"/plans/"+planID.String()+"/versions", "")
	if status != http.StatusCreated {
		t.Fatalf("create v2: %d (%s)", status, body)
	}
	var v2 struct{ ID uuid.UUID }
	mustJSON(t, body, &v2)
	if status, b := do(t, http.MethodPatch, srv.URL+"/plans/"+planID.String()+"/versions/"+v2.ID.String(),
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":2000}],"grace_days":7}`); status != http.StatusOK {
		t.Fatalf("update v2: %d (%s)", status, b)
	}
	if status, _ := post(t, srv.URL+"/plans/"+planID.String()+"/versions/"+v2.ID.String()+"/publish", ""); status != http.StatusOK {
		t.Fatalf("publish v2")
	}

	// v1's pinned data is unchanged (still 1000).
	_, v1body := get(t, srv.URL+"/versions/"+v1ID.String())
	if !strings.Contains(v1body, `"amount_minor":1000`) {
		t.Fatalf("v1 snapshot mutated: %s", v1body)
	}
	_, v2body := get(t, srv.URL+"/versions/"+v2.ID.String())
	if !strings.Contains(v2body, `"amount_minor":2000`) {
		t.Fatalf("v2 snapshot wrong: %s", v2body)
	}
}

func TestPublicListingExcludesHiddenAndDraft(t *testing.T) {
	deps, _ := newDeps(t)
	mod := catalog.New(deps)
	srv := adminServer(mod.Handler())
	defer srv.Close()

	// A published, public plan.
	createPublishedPlan(t, srv.URL, "visible", 1000)
	// A published but hidden plan (not made public).
	status, body := post(t, srv.URL+"/plans", `{"key":"hidden","name":"hidden"}`)
	if status != http.StatusCreated {
		t.Fatal("create hidden plan")
	}
	var hidden struct {
		Plan    struct{ ID uuid.UUID } `json:"plan"`
		Version struct{ ID uuid.UUID } `json:"version"`
	}
	mustJSON(t, body, &hidden)
	_, _ = do(t, http.MethodPatch, srv.URL+"/plans/"+hidden.Plan.ID.String()+"/versions/"+hidden.Version.ID.String(),
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":500}]}`)
	post(t, srv.URL+"/plans/"+hidden.Plan.ID.String()+"/versions/"+hidden.Version.ID.String()+"/publish", "")
	// A draft-only plan (never published).
	post(t, srv.URL+"/plans", `{"key":"draftonly","name":"draftonly"}`)

	// Public listing is open (no auth) and shows only the visible plan.
	pubSrv := httptest.NewServer(mod.Handler())
	defer pubSrv.Close()
	code, list := get(t, pubSrv.URL+"/public")
	if code != http.StatusOK {
		t.Fatalf("public list status %d", code)
	}
	if !strings.Contains(list, "visible") {
		t.Fatalf("public list missing the visible plan: %s", list)
	}
	if strings.Contains(list, "hidden") || strings.Contains(list, "draftonly") {
		t.Fatalf("public list leaked hidden/draft plans: %s", list)
	}
}

func TestGetPlanVersionPortReturnsFrozenSnapshot(t *testing.T) {
	deps, _ := newDeps(t)
	mod := catalog.New(deps)
	srv := adminServer(mod.Handler())
	defer srv.Close()

	_, v1ID := createPublishedPlan(t, srv.URL, "snap", 1500)

	info, err := mod.Port().GetPlanVersion(context.Background(), v1ID)
	if err != nil {
		t.Fatalf("GetPlanVersion: %v", err)
	}
	if info.PlanKey != "snap" || info.Status != "published" || info.Currency != "USD" {
		t.Fatalf("snapshot = %+v", info)
	}
	if len(info.Prices) != 1 || info.Prices[0].AmountMinor != 1500 {
		t.Fatalf("snapshot prices = %+v", info.Prices)
	}
	if info.GraceDays != 7 {
		t.Fatalf("snapshot grace = %d, want 7", info.GraceDays)
	}
	if v, ok := info.FeatureGrants["max_projects"]; !ok || int(v.(float64)) != 10 {
		t.Fatalf("snapshot feature grants = %+v", info.FeatureGrants)
	}
}

func TestPlanVersionPublishedEventEmitted(t *testing.T) {
	deps, bus := newDeps(t)
	mod := catalog.New(deps)
	var published int
	var mu sync.Mutex
	bus.Subscribe(ports.EventPlanVersionPublished, func(_ context.Context, _ events.Event) error {
		mu.Lock()
		published++
		mu.Unlock()
		return nil
	})
	relay := events.NewRelay(deps.Pool, bus, deps.Clock, deps.Logger, events.RelayConfig{})

	srv := adminServer(mod.Handler())
	defer srv.Close()
	createPublishedPlan(t, srv.URL, "evented", 1000)

	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("relay: %v", err)
	}
	mu.Lock()
	got := published
	mu.Unlock()
	if got < 1 {
		t.Fatalf("plan_version.published events = %d, want >= 1", got)
	}
}

func TestAdminRoutesRequireAuth(t *testing.T) {
	deps, _ := newDeps(t)
	mod := catalog.New(deps)
	// No principal injected.
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/plans", `{"key":"x","name":"x"}`); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create plan = %d, want 401", status)
	}
}

// --- helpers ---

func mustJSON(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
}
