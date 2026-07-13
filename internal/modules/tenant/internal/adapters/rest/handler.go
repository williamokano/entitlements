// Package rest is the tenant module's driving adapter: HTTP handlers for the
// tenant CRUD and lifecycle endpoints.
package rest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant/internal/service"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the tenant module's HTTP handler. Routes are prefix-relative; the
// composition root mounts them under /api/v1/tenants. It serves both the
// tenant-admin routes and the membership/invitation routes nested under a
// tenant id.
func New(svc *service.Service, mem *service.MembershipService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", createTenant(svc))
	mux.HandleFunc("GET /{id}", getTenant(svc))
	mux.HandleFunc("PATCH /{id}", updateTenant(svc))
	mux.HandleFunc("POST /{id}/suspend", lifecycle(svc.Suspend))
	mux.HandleFunc("POST /{id}/reactivate", lifecycle(svc.Reactivate))
	mux.HandleFunc("DELETE /{id}", lifecycle(svc.Delete))
	registerMembership(mux, mem)
	return mux
}

func createTenant(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Slug     string         `json:"slug"`
			Name     string         `json:"name"`
			Settings map[string]any `json:"settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		// Creating a tenant does not require authentication (a signup can
		// precede any account), but when a user *is* signed in they become the
		// tenant's owner — otherwise they would have no membership in the tenant
		// they just created, and no way to gain one.
		var creator *service.Creator
		if u, ok := optionalUser(r); ok {
			creator = &service.Creator{UserID: u.subject, Email: u.email}
		}
		info, err := svc.Create(r.Context(), body.Slug, body.Name, body.Settings, creator)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, toResponse(info))
	}
}

func getTenant(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		info, err := svc.GetByID(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toResponse(info))
	}
}

func updateTenant(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		var body struct {
			Name     string         `json:"name"`
			Settings map[string]any `json:"settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		info, err := svc.Update(r.Context(), id, body.Name, body.Settings)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toResponse(info))
	}
}

// lifecycle adapts a status-changing use case (suspend/reactivate/delete) to an
// HTTP handler returning 204 on success.
func lifecycle(action func(ctx context.Context, id uuid.UUID) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		if err := action(r.Context(), id); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid tenant id"))
		return uuid.Nil, false
	}
	return id, true
}

func toResponse(info ports.TenantInfo) map[string]any {
	return map[string]any{
		"id":     info.ID,
		"slug":   info.Slug,
		"name":   info.Name,
		"status": info.Status,
	}
}
