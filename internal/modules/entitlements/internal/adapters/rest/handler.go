// Package rest is the entitlements module's driving adapter: tenant-scoped HTTP
// handlers for reading a tenant's effective entitlements and managing its manual
// overrides, mounted under /api/v1/entitlements. Every route requires an
// authenticated principal.
package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the entitlements module's HTTP handler. Routes are prefix-relative;
// the composition root mounts them under /api/v1/entitlements (tenant-scoped).
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	// Override CRUD (admin). Registered alongside the catch-all GET /{key}; the
	// more specific /overrides patterns win under net/http precedence.
	mux.HandleFunc("POST /overrides", createOverride(svc))
	mux.HandleFunc("GET /overrides", listOverrides(svc))
	mux.HandleFunc("GET /overrides/{id}", getOverride(svc))
	mux.HandleFunc("PATCH /overrides/{id}", updateOverride(svc))
	mux.HandleFunc("DELETE /overrides/{id}", deleteOverride(svc))
	// Metered usage: consume/release against a limit, and read current usage.
	mux.HandleFunc("POST /consume", consume(svc))
	mux.HandleFunc("POST /release", release(svc))
	mux.HandleFunc("GET /usage", listUsage(svc))
	mux.HandleFunc("GET /usage/{key}", getUsage(svc))
	// Runtime reads.
	mux.HandleFunc("GET /", getAll(svc))
	mux.HandleFunc("GET /{key}", getOne(svc))
	return mux
}

// consumeBody is the consume/release payload: how many units of a feature.
type consumeBody struct {
	Key string `json:"key"`
	N   int64  `json:"n"`
}

// consume applies n units against a metered feature's limit. A hard-limit breach
// returns a typed quota-exceeded problem+json (HTTP 422).
func consume(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := usageTenant(w, r)
		if !ok {
			return
		}
		var body consumeBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		u, err := svc.ConsumeQuota(r.Context(), tenantID, body.Key, body.N)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, usageResponse(u))
	}
}

// release returns n units to a metered feature's current period.
func release(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := usageTenant(w, r)
		if !ok {
			return
		}
		var body consumeBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		u, err := svc.ReleaseQuota(r.Context(), tenantID, body.Key, body.N)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, usageResponse(u))
	}
}

// getUsage reports one metered feature's current usage.
func getUsage(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := usageTenant(w, r)
		if !ok {
			return
		}
		u, err := svc.GetUsage(r.Context(), tenantID, r.PathValue("key"))
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, usageResponse(u))
	}
}

// listUsage reports current usage for every active limit feature.
func listUsage(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := usageTenant(w, r)
		if !ok {
			return
		}
		us, err := svc.ListUsage(r.Context(), tenantID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(us))
		for _, u := range us {
			out = append(out, usageResponse(u))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"usage": out})
	}
}

// usageTenant enforces auth and resolves the tenant for a usage request.
func usageTenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if !requireAuth(w, r) {
		return uuid.Nil, false
	}
	tenantID, err := authctx.MustTenant(r.Context())
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
		return uuid.Nil, false
	}
	return tenantID, true
}

func usageResponse(u service.UsageView) map[string]any {
	resp := map[string]any{
		"key":       u.Key,
		"used":      u.Used,
		"period":    u.Period,
		"behavior":  u.Behavior,
		"unlimited": u.Unlimited,
	}
	if !u.Unlimited {
		resp["limit"] = u.Limit
	}
	return resp
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
			entry := map[string]any{"value": e.Value, "source": e.Source}
			if e.ExpiresAt != nil {
				entry["expires_at"] = e.ExpiresAt.UTC()
			}
			entitlements[key] = entry
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
		resp := map[string]any{"key": e.Key, "value": e.Value, "source": e.Source}
		if e.ExpiresAt != nil {
			resp["expires_at"] = e.ExpiresAt.UTC()
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

// overrideBody is the create/update payload. Actor is not accepted from the
// client; it is taken from the authenticated principal.
type overrideBody struct {
	FeatureKey string     `json:"feature_key"`
	Value      any        `json:"value"`
	Reason     string     `json:"reason"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

func createOverride(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body overrideBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.CreateOverride(r.Context(), service.OverrideInput{
			FeatureKey: body.FeatureKey,
			Value:      body.Value,
			Reason:     body.Reason,
			ExpiresAt:  body.ExpiresAt,
		})
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, overrideResponse(view))
	}
}

func updateOverride(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid override id"))
			return
		}
		var body overrideBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.UpdateOverride(r.Context(), id, service.OverrideInput{
			FeatureKey: body.FeatureKey,
			Value:      body.Value,
			Reason:     body.Reason,
			ExpiresAt:  body.ExpiresAt,
		})
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, overrideResponse(view))
	}
}

func deleteOverride(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid override id"))
			return
		}
		if err := svc.DeleteOverride(r.Context(), id); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getOverride(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid override id"))
			return
		}
		view, err := svc.GetOverride(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, overrideResponse(view))
	}
}

func listOverrides(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		views, err := svc.ListOverrides(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(views))
		for _, v := range views {
			out = append(out, overrideResponse(v))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"overrides": out})
	}
}

func overrideResponse(v service.OverrideView) map[string]any {
	resp := map[string]any{
		"id":          v.ID,
		"feature_key": v.FeatureKey,
		"value":       v.Value,
		"reason":      v.Reason,
		"actor":       v.Actor,
		"created_at":  v.CreatedAt.UTC(),
	}
	if v.ExpiresAt != nil {
		resp["expires_at"] = v.ExpiresAt.UTC()
	}
	return resp
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := authctx.PrincipalFromContext(r.Context()); !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return false
	}
	return true
}
