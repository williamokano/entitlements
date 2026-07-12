//go:build integration

package tenant_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/tenant"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newDeps(t *testing.T) (app.Deps, *events.Bus) {
	return newDepsClock(t, clock.System)
}

func newDepsClock(t *testing.T, clk clock.Clock) (app.Deps, *events.Bus) {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	bus := events.NewBus()
	deps := app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        bus,
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
	return deps, bus
}

// recordingHook records the order in which provisioning hooks run.
type recordingHook struct {
	name  string
	order *[]string
	mu    *sync.Mutex
}

func (h recordingHook) Name() string { return h.name }
func (h recordingHook) Provision(_ context.Context, _ uuid.UUID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, h.name)
	return nil
}

func TestProvisioningHooksRunInOrderAfterCommit(t *testing.T) {
	deps, bus := newDeps(t)
	ctx := context.Background()

	var mu sync.Mutex
	var order []string
	mod := tenant.New(deps, tenant.WithProvisioningHooks(
		recordingHook{name: "first", order: &order, mu: &mu},
		recordingHook{name: "second", order: &order, mu: &mu},
	))
	for _, s := range mod.Subscriptions() {
		bus.Subscribe(s.EventType, s.Handler)
	}
	relay := events.NewRelay(deps.Pool, bus, deps.Clock, deps.Logger, events.RelayConfig{})

	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	// A successful create emits tenant.created.
	if status, body := post(t, srv.URL, `{"slug":"hooked","name":"Hooked"}`); status != http.StatusCreated {
		t.Fatalf("create status = %d (%s), want 201", status, body)
	}
	// A duplicate slug conflicts and rolls back, so it emits no event — its
	// hooks must not run.
	if status, _ := post(t, srv.URL, `{"slug":"hooked","name":"Dup"}`); status != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", status)
	}

	if _, err := relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("hook order = %v, want exactly [first second] (once, in order)", got)
	}
}

func TestTenantCRUDEndpoints(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps, tenant.WithProvisioningHooks(tenant.NewLoggingHook(deps.Logger)))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	// Create.
	status, body := post(t, srv.URL, `{"slug":"acme","name":"Acme"}`)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d (%s), want 201", status, body)
	}
	var created struct {
		ID     uuid.UUID `json:"id"`
		Slug   string    `json:"slug"`
		Status string    `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Slug != "acme" || created.Status != "active" {
		t.Fatalf("created = %+v", created)
	}

	// Duplicate slug → 409.
	if status, _ := post(t, srv.URL, `{"slug":"acme","name":"Dup"}`); status != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", status)
	}

	// Get.
	if status, gb := get(t, srv.URL+"/"+created.ID.String()); status != http.StatusOK || !strings.Contains(gb, "acme") {
		t.Fatalf("get = %d %s, want 200 with slug", status, gb)
	}

	// Missing → 404.
	if status, _ := get(t, srv.URL+"/"+uuid.New().String()); status != http.StatusNotFound {
		t.Fatalf("missing get status = %d, want 404", status)
	}

	// Update name → 200.
	if status, _ := do(t, http.MethodPatch, srv.URL+"/"+created.ID.String(), `{"name":"Acme Renamed"}`); status != http.StatusOK {
		t.Fatalf("update status = %d, want 200", status)
	}

	// Suspend → 204, then get shows suspended.
	if status, _ := do(t, http.MethodPost, srv.URL+"/"+created.ID.String()+"/suspend", ""); status != http.StatusNoContent {
		t.Fatalf("suspend status = %d, want 204", status)
	}
	if _, gb := get(t, srv.URL+"/"+created.ID.String()); !strings.Contains(gb, "suspended") {
		t.Fatalf("after suspend, get body = %s, want suspended", gb)
	}

	// Delete → 204, then get is 404.
	if status, _ := do(t, http.MethodDelete, srv.URL+"/"+created.ID.String(), ""); status != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", status)
	}
	if status, _ := get(t, srv.URL+"/"+created.ID.String()); status != http.StatusNotFound {
		t.Fatalf("after delete, get status = %d, want 404", status)
	}
}

// --- HTTP helpers ---

func post(t *testing.T, url, body string) (int, string) { return do(t, http.MethodPost, url, body) }
func get(t *testing.T, url string) (int, string)        { return do(t, http.MethodGet, url, "") }

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
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
