// Package rest is the authorization module's driving adapter: HTTP handlers for
// role management and assignment, mounted under /api/v1/roles.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authorization/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// PermGuard builds middleware requiring a permission. The module supplies one
// backed by its Authorizer; rest stays free of the authorization package to
// avoid an import cycle.
type PermGuard func(permission string) httpx.Middleware

// Permissions used by the role-management routes themselves.
const (
	permRoleRead  = "role:read"
	permRoleWrite = "role:write"
)

// New returns the authorization module's HTTP handler. Each route is guarded by
// the permission it requires. Routes are prefix-relative; the composition root
// mounts them under /api/v1/roles behind auth + tenant resolution.
func New(svc *service.Service, guard PermGuard) http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern, perm string, h http.HandlerFunc) {
		mux.Handle(pattern, guard(perm)(h))
	}
	handle("GET /", permRoleRead, listRoles(svc))
	handle("POST /", permRoleWrite, createRole(svc))
	handle("GET /{id}", permRoleRead, getRole(svc))
	handle("PATCH /{id}", permRoleWrite, updateRole(svc))
	handle("DELETE /{id}", permRoleWrite, deleteRole(svc))
	handle("POST /{id}/assignments", permRoleWrite, assignRole(svc))
	handle("DELETE /{id}/assignments/{userId}", permRoleWrite, unassignRole(svc))
	return mux
}

func createRole(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		var body struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.CreateRole(r.Context(), tenantID, body.Name, body.Permissions)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, roleResponse(view))
	}
}

func listRoles(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		roles, err := svc.ListRoles(r.Context(), tenantID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(roles))
		for _, role := range roles {
			out = append(out, roleResponse(role))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"roles": out})
	}
}

func getRole(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		id, ok := pathID(w, r, "id", "role id")
		if !ok {
			return
		}
		view, err := svc.GetRole(r.Context(), tenantID, id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, roleResponse(view))
	}
}

func updateRole(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		id, ok := pathID(w, r, "id", "role id")
		if !ok {
			return
		}
		var body struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.UpdateRole(r.Context(), tenantID, id, body.Name, body.Permissions)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, roleResponse(view))
	}
}

func deleteRole(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		id, ok := pathID(w, r, "id", "role id")
		if !ok {
			return
		}
		if err := svc.DeleteRole(r.Context(), tenantID, id); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func assignRole(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		roleID, ok := pathID(w, r, "id", "role id")
		if !ok {
			return
		}
		var body struct {
			UserID uuid.UUID `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == uuid.Nil {
			httpx.WriteProblem(w, r, apperr.Validation("user_id is required"))
			return
		}
		if err := svc.Assign(r.Context(), tenantID, roleID, body.UserID); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func unassignRole(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant(w, r)
		if !ok {
			return
		}
		roleID, ok := pathID(w, r, "id", "role id")
		if !ok {
			return
		}
		userID, ok := pathID(w, r, "userId", "user id")
		if !ok {
			return
		}
		if err := svc.Unassign(r.Context(), tenantID, roleID, userID); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func tenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := authctx.MustTenant(r.Context())
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("tenant not specified"))
		return uuid.Nil, false
	}
	return id, true
}

func pathID(w http.ResponseWriter, r *http.Request, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid "+label))
		return uuid.Nil, false
	}
	return id, true
}

func roleResponse(v service.RoleView) map[string]any {
	return map[string]any{
		"id":          v.ID,
		"name":        v.Name,
		"permissions": v.Permissions,
		"system":      v.System,
		"created_at":  v.CreatedAt,
		"updated_at":  v.UpdatedAt,
	}
}
