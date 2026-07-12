//go:build integration

package authorization_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/authorization"
	pgadapter "github.com/williamokano/entitlements/internal/modules/authorization/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newDeps(t *testing.T) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	return app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clock.System,
		IDs:        id.UUIDv7{},
	}
}

// serverAs wraps the handler so it sees an authenticated user principal in the
// given tenant, standing in for the auth + tenant-resolution middleware.
func serverAs(h http.Handler, userID, tenantID uuid.UUID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithTenantID(r.Context(), tenantID)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: userID.String()})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}

// seed provisions the system roles for a tenant and returns the module + pool.
func seedTenant(t *testing.T, mod *authorization.Module, deps app.Deps, tenantID uuid.UUID) {
	t.Helper()
	if err := mod.SeedRolesHook().Provision(context.Background(), tenantID); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
}

// assignRole assigns a system role (by name) to a user directly, bootstrapping
// the initial permissions that the HTTP endpoints then require.
func assignRole(t *testing.T, deps app.Deps, tenantID, userID uuid.UUID, roleName string) {
	t.Helper()
	roles := pgadapter.NewRoles(deps.Pool)
	role, err := roles.GetByName(context.Background(), tenantID, roleName)
	if err != nil {
		t.Fatalf("get role %s: %v", roleName, err)
	}
	if err := pgadapter.NewAssignments(deps.Pool).Assign(context.Background(), uuid.New(), tenantID, userID, role.ID, time.Now().UTC()); err != nil {
		t.Fatalf("assign %s: %v", roleName, err)
	}
}

func TestSystemRolesSeededOnTenantProvisioning(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	tenantID := uuid.New()
	seedTenant(t, mod, deps, tenantID)

	roles, err := pgadapter.NewRoles(deps.Pool).List(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	names := map[string]bool{}
	for _, r := range roles {
		names[r.Name] = r.System
	}
	for _, want := range []string{"owner", "admin", "member"} {
		if !names[want] {
			t.Fatalf("system role %q not seeded (roles: %v)", want, names)
		}
	}

	// Seeding is idempotent.
	seedTenant(t, mod, deps, tenantID)
	roles2, _ := pgadapter.NewRoles(deps.Pool).List(context.Background(), tenantID)
	if len(roles2) != len(roles) {
		t.Fatalf("re-seed changed role count %d -> %d", len(roles), len(roles2))
	}
}

func TestSystemRoleDeleteOrEditRejected409(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	tenantID, userID := uuid.New(), uuid.New()
	seedTenant(t, mod, deps, tenantID)
	assignRole(t, deps, tenantID, userID, "owner") // owner has *:*

	srv := serverAs(mod.Handler(), userID, tenantID)
	defer srv.Close()

	ownerID := roleIDByName(t, deps, tenantID, "owner")
	if status, _ := do(t, http.MethodDelete, srv.URL+"/"+ownerID.String(), ""); status != http.StatusConflict {
		t.Fatalf("delete system role status = %d, want 409", status)
	}
	if status, _ := do(t, http.MethodPatch, srv.URL+"/"+ownerID.String(), `{"permissions":["role:read"]}`); status != http.StatusConflict {
		t.Fatalf("edit system role status = %d, want 409", status)
	}
}

func TestCustomRoleCreatedAtRuntimeGrantsAccess(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	tenantID, adminUser := uuid.New(), uuid.New()
	seedTenant(t, mod, deps, tenantID)
	assignRole(t, deps, tenantID, adminUser, "admin") // admin has role:write

	admin := serverAs(mod.Handler(), adminUser, tenantID)
	defer admin.Close()

	// Admin creates a custom role at runtime (no deploy).
	status, body := post(t, admin.URL, `{"name":"support","permissions":["ticket:read","ticket:write"]}`)
	if status != http.StatusCreated {
		t.Fatalf("create role status = %d (%s), want 201", status, body)
	}
	// The new role is listable.
	if _, list := get(t, admin.URL); !strings.Contains(list, "support") {
		t.Fatalf("custom role not listed: %s", list)
	}
}

func TestUnassignRevokesAccess403(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	tenantID, userID := uuid.New(), uuid.New()
	seedTenant(t, mod, deps, tenantID)
	assignRole(t, deps, tenantID, userID, "admin")

	srv := serverAs(mod.Handler(), userID, tenantID)
	defer srv.Close()

	// With admin, listing works.
	if status, _ := get(t, srv.URL); status != http.StatusOK {
		t.Fatalf("pre-unassign list = %d, want 200", status)
	}
	// Unassign admin, then access is denied.
	adminRoleID := roleIDByName(t, deps, tenantID, "admin")
	if err := pgadapter.NewAssignments(deps.Pool).Unassign(context.Background(), tenantID, userID, adminRoleID); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if status, _ := get(t, srv.URL); status != http.StatusForbidden {
		t.Fatalf("post-unassign list = %d, want 403", status)
	}
}

func TestSameUserDifferentRolesPerTenant(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	tenantA, tenantB, userID := uuid.New(), uuid.New(), uuid.New()
	seedTenant(t, mod, deps, tenantA)
	seedTenant(t, mod, deps, tenantB)
	assignRole(t, deps, tenantA, userID, "admin")  // write in A
	assignRole(t, deps, tenantB, userID, "member") // read-only in B

	inA := serverAs(mod.Handler(), userID, tenantA)
	defer inA.Close()
	inB := serverAs(mod.Handler(), userID, tenantB)
	defer inB.Close()

	// Admin in A can create a role.
	if status, _ := post(t, inA.URL, `{"name":"x","permissions":["a:read"]}`); status != http.StatusCreated {
		t.Fatalf("create in A = %d, want 201", status)
	}
	// Member in B cannot (role:write denied), but can read.
	if status, _ := post(t, inB.URL, `{"name":"y","permissions":["a:read"]}`); status != http.StatusForbidden {
		t.Fatalf("create in B = %d, want 403", status)
	}
	if status, _ := get(t, inB.URL); status != http.StatusOK {
		t.Fatalf("read in B = %d, want 200 (member has role:read)", status)
	}
}

func TestRequirePermissionMiddlewareDenies403WithProblemJSON(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	tenantID, userID := uuid.New(), uuid.New()
	seedTenant(t, mod, deps, tenantID)
	assignRole(t, deps, tenantID, userID, "member") // read-only

	srv := serverAs(mod.Handler(), userID, tenantID)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/", "application/json", strings.NewReader(`{"name":"z","permissions":["a:read"]}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Fatalf("content-type = %q, want application/problem+json", ct)
	}
}

func TestUnauthenticatedRoleRouteDenied(t *testing.T) {
	deps := newDeps(t)
	mod := authorization.New(deps)
	// Tenant set but no principal.
	tenantID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mod.Handler().ServeHTTP(w, r.WithContext(authctx.WithTenantID(r.Context(), tenantID)))
	}))
	defer srv.Close()
	if status, _ := get(t, srv.URL); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated role list = %d, want 401", status)
	}
}

// --- helpers ---

func roleIDByName(t *testing.T, deps app.Deps, tenantID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	role, err := pgadapter.NewRoles(deps.Pool).GetByName(context.Background(), tenantID, name)
	if err != nil {
		t.Fatalf("get role %s: %v", name, err)
	}
	return role.ID
}

func post(t *testing.T, url, body string) (int, string) { return do(t, http.MethodPost, url+"/", body) }
func get(t *testing.T, url string) (int, string)        { return do(t, http.MethodGet, url+"/", "") }

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
