// Package rest is the example module's driving adapter: HTTP handlers that call
// the service and render responses.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/example/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the example module's HTTP handler. Routes are prefix-relative;
// the composition root mounts them under /api/v1/example.
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /things", createThing(svc))
	mux.HandleFunc("GET /things/{id}", getThing(svc))
	return mux
}

func createThing(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			httpx.WriteProblem(w, r, apperr.Validation("name is required"))
			return
		}
		tenant, _ := authctx.TenantID(r.Context())

		thing, err := svc.CreateThing(r.Context(), tenant, body.Name)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"id":   thing.ID,
			"name": thing.Name,
		})
	}
}

func getThing(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid id"))
			return
		}
		name, err := svc.GetThingName(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "name": name})
	}
}
