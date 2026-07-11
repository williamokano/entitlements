package tenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
)

// fakeReader is an in-memory ports.TenantReader for middleware unit tests.
type fakeReader struct {
	byID   map[uuid.UUID]ports.TenantInfo
	bySlug map[string]ports.TenantInfo
}

func newFakeReader(infos ...ports.TenantInfo) *fakeReader {
	f := &fakeReader{byID: map[uuid.UUID]ports.TenantInfo{}, bySlug: map[string]ports.TenantInfo{}}
	for _, i := range infos {
		f.byID[i.ID] = i
		f.bySlug[i.Slug] = i
	}
	return f
}

func (f *fakeReader) GetByID(_ context.Context, id uuid.UUID) (ports.TenantInfo, error) {
	if i, ok := f.byID[id]; ok {
		return i, nil
	}
	return ports.TenantInfo{}, apperr.NotFound("tenant not found")
}

func (f *fakeReader) GetBySlug(_ context.Context, slug string) (ports.TenantInfo, error) {
	if i, ok := f.bySlug[slug]; ok {
		return i, nil
	}
	return ports.TenantInfo{}, apperr.NotFound("tenant not found")
}

// probe records the tenant the middleware resolved into context.
func probe(resolved *uuid.UUID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := authctx.TenantID(r.Context())
		*resolved = id
		w.WriteHeader(http.StatusOK)
	})
}

func active(slug string) (uuid.UUID, ports.TenantInfo) {
	id := uuid.New()
	return id, ports.TenantInfo{ID: id, Slug: slug, Status: ports.StatusActive}
}

func TestTenantResolutionPrecedenceClaimOverHeaderOverSubdomain(t *testing.T) {
	claimID, claimT := active("claimco")
	headerID, headerT := active("headerco")
	_, subT := active("subco")
	reader := newFakeReader(claimT, headerT, subT)

	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(reader)(probe(&resolved))

	// All three present → claim wins.
	req := httptest.NewRequest(http.MethodGet, "http://subco.example.com/x", nil)
	req.Header.Set(tenant.HeaderTenantID, headerID.String())
	req = req.WithContext(authctx.WithTenantClaim(req.Context(), claimID.String()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || resolved != claimID {
		t.Fatalf("claim precedence: code=%d resolved=%s, want 200 %s", rec.Code, resolved, claimID)
	}

	// Header + subdomain (no claim) → header wins.
	resolved = uuid.Nil
	req = httptest.NewRequest(http.MethodGet, "http://subco.example.com/x", nil)
	req.Header.Set(tenant.HeaderTenantID, headerID.String())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if resolved != headerID {
		t.Fatalf("header precedence: resolved=%s, want %s", resolved, headerID)
	}
}

func TestTenantFromHeader(t *testing.T) {
	id, info := active("acme")
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(newFakeReader(info))(probe(&resolved))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(tenant.HeaderTenantID, id.String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || resolved != id {
		t.Fatalf("header source: code=%d resolved=%s, want 200 %s", rec.Code, resolved, id)
	}
}

func TestTenantFromSubdomain(t *testing.T) {
	id, info := active("acme")
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(newFakeReader(info))(probe(&resolved))

	req := httptest.NewRequest(http.MethodGet, "http://acme.example.com/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || resolved != id {
		t.Fatalf("subdomain source: code=%d resolved=%s, want 200 %s", rec.Code, resolved, id)
	}
}

func TestSuspendedTenantRejected403(t *testing.T) {
	id := uuid.New()
	reader := newFakeReader(ports.TenantInfo{ID: id, Slug: "susp", Status: ports.StatusSuspended})
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(reader)(probe(&resolved))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(tenant.HeaderTenantID, id.String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("suspended tenant code = %d, want 403", rec.Code)
	}
}

func TestDeletedOrUnknownTenantRejected(t *testing.T) {
	reader := newFakeReader() // empty: reader excludes deleted/unknown → NotFound
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(reader)(probe(&resolved))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(tenant.HeaderTenantID, uuid.New().String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown tenant code = %d, want 404", rec.Code)
	}
}

func TestMissingTenantOnScopedRouteReturns400(t *testing.T) {
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(newFakeReader())(probe(&resolved))

	req := httptest.NewRequest(http.MethodGet, "/x", nil) // no claim, header, or subdomain
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing tenant code = %d, want 400", rec.Code)
	}
}

func TestIPHostIsNotTreatedAsSubdomain(t *testing.T) {
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(newFakeReader())(probe(&resolved))

	// An IP literal host (as used by httptest) must not have its octets read as
	// a <slug>.<domain>; with no other tenant source this is a plain 400.
	for _, host := range []string{"127.0.0.1:8080", "127.0.0.1", "10.0.0.5"} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("host %s code = %d, want 400 (no tenant, not a subdomain lookup)", host, rec.Code)
		}
	}
}

func TestExemptPrefixBypassesResolution(t *testing.T) {
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(newFakeReader(), tenant.WithExempt("/healthz", "/api/v1/tenants"))(probe(&resolved))

	for _, path := range []string{"/healthz", "/api/v1/tenants", "/api/v1/tenants/abc"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("exempt %s code = %d, want 200 (no tenant required)", path, rec.Code)
		}
	}
}

func TestWithSystemContextBypassesTenantGuard(t *testing.T) {
	var resolved uuid.UUID
	h := tenant.ResolveMiddleware(newFakeReader())(probe(&resolved))

	// Internal (system) context: passes without a tenant.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(authctx.WithSystemContext(req.Context()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("system context code = %d, want 200 (guard bypassed)", rec.Code)
	}

	// A normal request (no system context) is still rejected — the bypass can't
	// be reached from a request.
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("normal request code = %d, want 400 (not bypassed)", rec.Code)
	}
}
