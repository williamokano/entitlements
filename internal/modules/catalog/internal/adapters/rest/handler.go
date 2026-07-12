// Package rest is the catalog module's driving adapter: admin CRUD for plans and
// versions plus a public plan listing, mounted under /api/v1/catalog.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/catalog/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/catalog/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the catalog module's HTTP handler. Admin routes require an
// authenticated principal; the public listing is open. Routes are
// prefix-relative; the composition root mounts them under /api/v1/catalog
// (exempt from tenant resolution — the catalog is global).
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /plans", createPlan(svc))
	mux.HandleFunc("GET /plans", listPlans(svc))
	mux.HandleFunc("GET /plans/{id}", getPlan(svc))
	mux.HandleFunc("POST /plans/{id}/public", setPublic(svc))
	mux.HandleFunc("POST /plans/{id}/archive", archivePlan(svc))
	mux.HandleFunc("POST /plans/{id}/versions", createVersion(svc))
	mux.HandleFunc("PATCH /plans/{id}/versions/{vid}", updateVersion(svc))
	mux.HandleFunc("POST /plans/{id}/versions/{vid}/publish", publishVersion(svc))
	mux.HandleFunc("GET /versions/{vid}", getVersion(svc))
	mux.HandleFunc("GET /public", listPublic(svc))
	registerAddons(mux, svc)
	return mux
}

type versionBody struct {
	Currency      string             `json:"currency"`
	Prices        []domain.Price     `json:"prices"`
	Trial         domain.TrialConfig `json:"trial"`
	GraceDays     int                `json:"grace_days"`
	FeatureGrants map[string]any     `json:"feature_grants"`
}

func createPlan(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		var body struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		plan, version, err := svc.CreatePlan(r.Context(), body.Key, body.Name)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"plan":    planResponse(plan),
			"version": versionResponse(version),
		})
	}
}

func listPlans(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		plans, err := svc.ListPlans(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(plans))
		for _, p := range plans {
			out = append(out, planResponse(p))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"plans": out})
	}
}

func getPlan(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "plan id")
		if !ok {
			return
		}
		plan, versions, err := svc.GetPlan(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		vs := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			vs = append(vs, versionResponse(v))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": planResponse(plan), "versions": vs})
	}
}

func setPublic(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "plan id")
		if !ok {
			return
		}
		var body struct {
			Public bool `json:"public"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		plan, err := svc.SetPlanPublic(r.Context(), id, body.Public)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, planResponse(plan))
	}
}

func archivePlan(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "plan id")
		if !ok {
			return
		}
		if err := svc.ArchivePlan(r.Context(), id); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func createVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "plan id")
		if !ok {
			return
		}
		view, err := svc.CreateNewVersion(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, versionResponse(view))
	}
}

func updateVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		planID, ok := pathID(w, r, "id", "plan id")
		if !ok {
			return
		}
		vid, ok := pathID(w, r, "vid", "version id")
		if !ok {
			return
		}
		var body versionBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.UpdateDraftVersion(r.Context(), planID, vid, service.VersionContent{
			Currency:      body.Currency,
			Prices:        body.Prices,
			Trial:         body.Trial,
			GraceDays:     body.GraceDays,
			FeatureGrants: body.FeatureGrants,
		})
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, versionResponse(view))
	}
}

func publishVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		planID, ok := pathID(w, r, "id", "plan id")
		if !ok {
			return
		}
		vid, ok := pathID(w, r, "vid", "version id")
		if !ok {
			return
		}
		view, err := svc.PublishVersion(r.Context(), planID, vid)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, versionResponse(view))
	}
}

func getVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid, ok := pathID(w, r, "vid", "version id")
		if !ok {
			return
		}
		info, err := svc.GetPlanVersion(r.Context(), vid)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		prices := make([]map[string]any, 0, len(info.Prices))
		for _, p := range info.Prices {
			prices = append(prices, map[string]any{"cycle": p.Cycle, "amount_minor": p.AmountMinor})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"id":             info.ID,
			"plan_id":        info.PlanID,
			"plan_key":       info.PlanKey,
			"version":        info.Version,
			"status":         info.Status,
			"currency":       info.Currency,
			"prices":         prices,
			"trial":          map[string]any{"enabled": info.TrialEnabled, "days": info.TrialDays, "card_required": info.CardRequired},
			"grace_days":     info.GraceDays,
			"feature_grants": info.FeatureGrants,
		})
	}
}

func listPublic(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plans, err := svc.ListPublicPlans(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(plans))
		for _, p := range plans {
			out = append(out, map[string]any{
				"plan":    planResponse(p.Plan),
				"version": versionResponse(p.Version),
			})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"plans": out})
	}
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := authctx.PrincipalFromContext(r.Context()); !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid "+label))
		return uuid.Nil, false
	}
	return id, true
}

func planResponse(p service.PlanView) map[string]any {
	return map[string]any{"id": p.ID, "key": p.Key, "name": p.Name, "status": p.Status, "public": p.Public}
}

func versionResponse(v service.VersionView) map[string]any {
	prices := make([]map[string]any, 0, len(v.Prices))
	for _, p := range v.Prices {
		prices = append(prices, map[string]any{"cycle": p.Cycle, "amount_minor": p.AmountMinor})
	}
	return map[string]any{
		"id":             v.ID,
		"plan_id":        v.PlanID,
		"version":        v.Version,
		"status":         v.Status,
		"currency":       v.Currency,
		"prices":         prices,
		"trial":          map[string]any{"enabled": v.Trial.Enabled, "days": v.Trial.Days, "card_required": v.Trial.CardRequired},
		"grace_days":     v.GraceDays,
		"feature_grants": v.FeatureGrants,
		"published_at":   v.PublishedAt,
	}
}
