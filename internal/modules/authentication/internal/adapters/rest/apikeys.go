package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// NewAPIKeys returns the tenant-scoped API-key management handler. Routes are
// prefix-relative; the composition root mounts them under /api/v1/api-keys
// behind tenant resolution and RequireAuth.
func NewAPIKeys(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", createAPIKey(svc))
	mux.HandleFunc("GET /", listAPIKeys(svc))
	mux.HandleFunc("DELETE /{id}", revokeAPIKey(svc))
	return mux
}

func createAPIKey(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := authctx.MustTenant(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
			return
		}
		var body struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, secret, err := svc.CreateAPIKey(r.Context(), tenantID, body.Name, body.Scopes)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		// The secret is returned exactly once, here, and never again.
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"id":         view.ID,
			"name":       view.Name,
			"prefix":     view.Prefix,
			"scopes":     view.Scopes,
			"api_key":    secret,
			"created_at": view.CreatedAt,
		})
	}
}

func listAPIKeys(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := authctx.MustTenant(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
			return
		}
		keys, err := svc.ListAPIKeys(r.Context(), tenantID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, map[string]any{
				"id":           k.ID,
				"name":         k.Name,
				"prefix":       k.Prefix,
				"scopes":       k.Scopes,
				"last_used_at": k.LastUsedAt,
				"created_at":   k.CreatedAt,
			})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"api_keys": out})
	}
}

func revokeAPIKey(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := authctx.MustTenant(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
			return
		}
		keyID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid api key id"))
			return
		}
		if err := svc.RevokeAPIKey(r.Context(), tenantID, keyID); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
