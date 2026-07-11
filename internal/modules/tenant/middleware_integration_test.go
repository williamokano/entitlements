//go:build integration

package tenant_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant"
	"github.com/williamokano/entitlements/internal/platform/authctx"
)

// TestResolveMiddlewareWithRealReader exercises the middleware against the real
// tenant module (service + Postgres), not a fake reader.
func TestResolveMiddlewareWithRealReader(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps)

	// Create an active tenant and a suspended one through the module's HTTP API.
	adminSrv := httptest.NewServer(mod.Handler())
	defer adminSrv.Close()

	_, activeBody := post(t, adminSrv.URL, `{"slug":"real-active","name":"Active"}`)
	activeID := idOf(t, activeBody)

	_, suspBody := post(t, adminSrv.URL, `{"slug":"real-susp","name":"Susp"}`)
	suspID := idOf(t, suspBody)
	if status, _ := do(t, http.MethodPost, adminSrv.URL+"/"+suspID.String()+"/suspend", ""); status != http.StatusNoContent {
		t.Fatalf("suspend setup: status %d", status)
	}

	var resolved uuid.UUID
	scoped := tenant.ResolveMiddleware(mod.Port())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := authctx.MustTenant(r.Context())
		if err != nil {
			t.Errorf("MustTenant in scoped handler: %v", err)
		}
		resolved = id
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(scoped)
	defer srv.Close()

	// Active tenant via header → 200, resolved.
	if status := doWithTenant(t, srv.URL, activeID); status != http.StatusOK || resolved != activeID {
		t.Fatalf("active: status=%d resolved=%s, want 200 %s", status, resolved, activeID)
	}
	// Suspended → 403.
	if status := doWithTenant(t, srv.URL, suspID); status != http.StatusForbidden {
		t.Fatalf("suspended: status=%d, want 403", status)
	}
	// Unknown → 404.
	if status := doWithTenant(t, srv.URL, uuid.New()); status != http.StatusNotFound {
		t.Fatalf("unknown: status=%d, want 404", status)
	}
}

func doWithTenant(t *testing.T, url string, tenantID uuid.UUID) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set(tenant.HeaderTenantID, tenantID.String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func idOf(t *testing.T, body string) uuid.UUID {
	t.Helper()
	var out struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("parse id from %q: %v", body, err)
	}
	return out.ID
}
