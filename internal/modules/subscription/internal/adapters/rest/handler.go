// Package rest is the subscription module's driving adapter: HTTP handlers for
// the tenant's subscription lifecycle, mounted under /api/v1/subscription.
package rest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/subscription/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the subscription module's HTTP handler. Routes are prefix-relative;
// the composition root mounts them under /api/v1/subscription (tenant-scoped).
// Every route requires an authenticated principal.
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", createSubscription(svc))
	mux.HandleFunc("GET /", getSubscription(svc))
	mux.HandleFunc("POST /cancel", cancelSubscription(svc))
	mux.HandleFunc("POST /reactivate", lifecycle(svc.Reactivate))
	mux.HandleFunc("POST /pause", lifecycle(svc.Pause))
	mux.HandleFunc("POST /resume", lifecycle(svc.Resume))
	mux.HandleFunc("POST /change-plan", changePlan(svc))
	mux.HandleFunc("POST /scheduled-change/cancel", cancelScheduledChange(svc))
	mux.HandleFunc("POST /addons", attachAddon(svc))
	mux.HandleFunc("DELETE /addons/{vid}", detachAddon(svc))
	return mux
}

func changePlan(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body struct {
			PlanVersionID uuid.UUID `json:"plan_version_id"`
			Cycle         string    `json:"cycle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.ChangePlan(r.Context(), body.PlanVersionID, body.Cycle)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

func cancelScheduledChange(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		view, err := svc.CancelScheduledChange(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

func attachAddon(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body struct {
			AddonVersionID uuid.UUID `json:"addon_version_id"`
			Quantity       int       `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if body.Quantity == 0 {
			body.Quantity = 1
		}
		view, err := svc.AttachAddon(r.Context(), body.AddonVersionID, body.Quantity)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

func detachAddon(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		vid, err := uuid.Parse(r.PathValue("vid"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid addon version id"))
			return
		}
		view, err := svc.DetachAddon(r.Context(), vid)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

func createSubscription(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body struct {
			PlanVersionID uuid.UUID `json:"plan_version_id"`
			Cycle         string    `json:"cycle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.Create(r.Context(), body.PlanVersionID, body.Cycle)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, subscriptionResponse(view))
	}
}

func getSubscription(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		view, err := svc.GetForTenant(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

func cancelSubscription(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body struct {
			Immediate bool   `json:"immediate"`
			Reason    string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		view, err := svc.Cancel(r.Context(), body.Immediate, body.Reason)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

// lifecycle adapts a reason-taking transition (reactivate/pause/resume) to an
// HTTP handler.
func lifecycle(action func(ctx context.Context, reason string) (service.View, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		view, err := action(r.Context(), body.Reason)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, subscriptionResponse(view))
	}
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := authctx.PrincipalFromContext(r.Context()); !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return false
	}
	return true
}

func subscriptionResponse(v service.View) map[string]any {
	out := map[string]any{
		"id":                   v.ID,
		"plan_version_id":      v.PlanVersionID,
		"billing_cycle":        v.BillingCycle,
		"status":               v.Status,
		"current_period_start": v.CurrentPeriodStart,
		"current_period_end":   v.CurrentPeriodEnd,
		"cancel_at_period_end": v.CancelAtPeriodEnd,
	}
	if v.TrialEndsAt != "" {
		out["trial_ends_at"] = v.TrialEndsAt
	}
	if v.ScheduledChange != nil {
		out["scheduled_change"] = map[string]any{
			"plan_version_id": v.ScheduledChange.PlanVersionID,
			"billing_cycle":   v.ScheduledChange.BillingCycle,
		}
	}
	if v.Addons != nil {
		addons := make([]map[string]any, 0, len(v.Addons))
		for _, a := range v.Addons {
			addons = append(addons, map[string]any{"addon_version_id": a.AddonVersionID, "quantity": a.Quantity})
		}
		out["addons"] = addons
	}
	return out
}
