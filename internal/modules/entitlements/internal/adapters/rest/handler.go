// Package rest is the entitlements module's driving adapter: tenant-scoped HTTP
// handlers for reading a tenant's effective entitlements, mounted under
// /api/v1/entitlements. Every route requires an authenticated principal.
package rest

import (
	"net/http"

	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the entitlements module's HTTP handler. Routes are prefix-relative;
// the composition root mounts them under /api/v1/entitlements (tenant-scoped).
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", getAll(svc))
	mux.HandleFunc("GET /{key}", getOne(svc))
	return mux
}

// getAll returns the tenant's whole effective set in one call.
func getAll(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		tenantID, err := authctx.MustTenant(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
			return
		}
		set, err := svc.GetAll(r.Context(), tenantID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		entitlements := make(map[string]any, len(set))
		for key, e := range set {
			entitlements[key] = map[string]any{"value": e.Value, "source": e.Source}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"entitlements": entitlements})
	}
}

// getOne returns a single feature's effective entitlement.
func getOne(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		tenantID, err := authctx.MustTenant(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
			return
		}
		e, err := svc.Get(r.Context(), tenantID, r.PathValue("key"))
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"key": e.Key, "value": e.Value, "source": e.Source})
	}
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := authctx.PrincipalFromContext(r.Context()); !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return false
	}
	return true
}
