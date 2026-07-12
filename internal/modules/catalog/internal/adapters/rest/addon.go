package rest

import (
	"encoding/json"
	"net/http"

	"github.com/williamokano/entitlements/internal/modules/catalog/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/catalog/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// registerAddons wires the addon routes onto the catalog module's mux. Admin
// routes require an authenticated principal; the frozen snapshot read is open.
func registerAddons(mux *http.ServeMux, svc *service.Service) {
	mux.HandleFunc("POST /addons", createAddon(svc))
	mux.HandleFunc("GET /addons", listAddons(svc))
	mux.HandleFunc("GET /addons/{id}", getAddon(svc))
	mux.HandleFunc("POST /addons/{id}/archive", archiveAddon(svc))
	mux.HandleFunc("POST /addons/{id}/versions", createAddonVersion(svc))
	mux.HandleFunc("PATCH /addons/{id}/versions/{vid}", updateAddonVersion(svc))
	mux.HandleFunc("POST /addons/{id}/versions/{vid}/publish", publishAddonVersion(svc))
	mux.HandleFunc("GET /addon-versions/{vid}", getAddonVersion(svc))
}

type addonVersionBody struct {
	Currency           string         `json:"currency"`
	Prices             []domain.Price `json:"prices"`
	QuantityAllowed    bool           `json:"quantity_allowed"`
	CompatiblePlanKeys []string       `json:"compatible_plan_keys"`
	Deltas             []domain.Delta `json:"deltas"`
}

func createAddon(svc *service.Service) http.HandlerFunc {
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
		addon, version, err := svc.CreateAddon(r.Context(), body.Key, body.Name)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"addon":   addonResponse(addon),
			"version": addonVersionResponse(version),
		})
	}
}

func listAddons(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		addons, err := svc.ListAddons(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(addons))
		for _, a := range addons {
			out = append(out, addonResponse(a))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"addons": out})
	}
}

func getAddon(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "addon id")
		if !ok {
			return
		}
		addon, versions, err := svc.GetAddon(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		vs := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			vs = append(vs, addonVersionResponse(v))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"addon": addonResponse(addon), "versions": vs})
	}
}

func archiveAddon(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "addon id")
		if !ok {
			return
		}
		if err := svc.ArchiveAddon(r.Context(), id); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func createAddonVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, ok := pathID(w, r, "id", "addon id")
		if !ok {
			return
		}
		view, err := svc.CreateNewAddonVersion(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, addonVersionResponse(view))
	}
}

func updateAddonVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		addonID, ok := pathID(w, r, "id", "addon id")
		if !ok {
			return
		}
		vid, ok := pathID(w, r, "vid", "version id")
		if !ok {
			return
		}
		var body addonVersionBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.UpdateDraftAddonVersion(r.Context(), addonID, vid, service.AddonVersionContent{
			Currency:           body.Currency,
			Prices:             body.Prices,
			QuantityAllowed:    body.QuantityAllowed,
			CompatiblePlanKeys: body.CompatiblePlanKeys,
			Deltas:             body.Deltas,
		})
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, addonVersionResponse(view))
	}
}

func publishAddonVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		addonID, ok := pathID(w, r, "id", "addon id")
		if !ok {
			return
		}
		vid, ok := pathID(w, r, "vid", "version id")
		if !ok {
			return
		}
		view, err := svc.PublishAddonVersion(r.Context(), addonID, vid)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, addonVersionResponse(view))
	}
}

func getAddonVersion(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid, ok := pathID(w, r, "vid", "version id")
		if !ok {
			return
		}
		info, err := svc.GetAddonVersion(r.Context(), vid)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		prices := make([]map[string]any, 0, len(info.Prices))
		for _, p := range info.Prices {
			prices = append(prices, map[string]any{"cycle": p.Cycle, "amount_minor": p.AmountMinor})
		}
		deltas := make([]map[string]any, 0, len(info.Deltas))
		for _, d := range info.Deltas {
			deltas = append(deltas, deltaResponse(d.FeatureKey, d.Kind, d.Amount, d.Value))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"id":                   info.ID,
			"addon_id":             info.AddonID,
			"addon_key":            info.AddonKey,
			"version":              info.Version,
			"status":               info.Status,
			"currency":             info.Currency,
			"prices":               prices,
			"quantity_allowed":     info.QuantityAllowed,
			"compatible_plan_keys": info.CompatiblePlanKeys,
			"deltas":               deltas,
		})
	}
}

func addonResponse(a service.AddonView) map[string]any {
	return map[string]any{"id": a.ID, "key": a.Key, "name": a.Name, "status": a.Status}
}

func addonVersionResponse(v service.AddonVersionView) map[string]any {
	prices := make([]map[string]any, 0, len(v.Prices))
	for _, p := range v.Prices {
		prices = append(prices, map[string]any{"cycle": p.Cycle, "amount_minor": p.AmountMinor})
	}
	deltas := make([]map[string]any, 0, len(v.Deltas))
	for _, d := range v.Deltas {
		deltas = append(deltas, deltaResponse(d.FeatureKey, string(d.Kind), d.Amount, d.Value))
	}
	return map[string]any{
		"id":                   v.ID,
		"addon_id":             v.AddonID,
		"version":              v.Version,
		"status":               v.Status,
		"currency":             v.Currency,
		"prices":               prices,
		"quantity_allowed":     v.QuantityAllowed,
		"compatible_plan_keys": v.CompatiblePlanKeys,
		"deltas":               deltas,
	}
}

func deltaResponse(featureKey, kind string, amount int64, value any) map[string]any {
	d := map[string]any{"feature_key": featureKey, "kind": kind}
	if value != nil {
		d["value"] = value
	} else {
		d["amount"] = amount
	}
	return d
}
