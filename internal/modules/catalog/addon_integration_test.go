//go:build integration

package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/catalog"
)

// createPublishedAddon creates an addon, sets v1 content (compatible with the
// given plan keys, with a delta), publishes it. Returns addon id and v1 id.
func createPublishedAddon(t *testing.T, url, key string, compatible []string) (addonID, v1ID uuid.UUID) {
	t.Helper()
	status, body := post(t, url+"/addons", `{"key":"`+key+`","name":"`+key+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create addon: %d (%s)", status, body)
	}
	var created struct {
		Addon   struct{ ID uuid.UUID } `json:"addon"`
		Version struct{ ID uuid.UUID } `json:"version"`
	}
	mustJSON(t, body, &created)
	addonID, v1ID = created.Addon.ID, created.Version.ID

	compat := `["` + strings.Join(compatible, `","`) + `"]`
	content := `{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":300}],"quantity_allowed":true,"compatible_plan_keys":` + compat + `,"deltas":[{"feature_key":"max_projects","kind":"limit_delta","amount":10}]}`
	if status, b := do(t, http.MethodPatch, url+"/addons/"+addonID.String()+"/versions/"+v1ID.String(), content); status != http.StatusOK {
		t.Fatalf("update addon v1: %d (%s)", status, b)
	}
	if status, b := post(t, url+"/addons/"+addonID.String()+"/versions/"+v1ID.String()+"/publish", ""); status != http.StatusOK {
		t.Fatalf("publish addon v1: %d (%s)", status, b)
	}
	return addonID, v1ID
}

func TestAddonCRUDAndVersionImmutability(t *testing.T) {
	deps, _ := newDeps(t)
	mod := catalog.New(deps)
	srv := adminServer(mod.Handler())
	defer srv.Close()

	// Admin routes need auth.
	noAuth := httptest.NewServer(mod.Handler())
	defer noAuth.Close()
	if status, _ := post(t, noAuth.URL+"/addons", `{"key":"x","name":"x"}`); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated addon create, want 401")
	}

	addonID, v1ID := createPublishedAddon(t, srv.URL, "extra-seats", []string{"pro"})

	// Editing the published v1 is rejected.
	if status, _ := do(t, http.MethodPatch, srv.URL+"/addons/"+addonID.String()+"/versions/"+v1ID.String(),
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":999}],"compatible_plan_keys":["pro"]}`); status != http.StatusConflict {
		t.Fatalf("edit published addon version, want 409")
	}

	// Open v2, change price, publish; v1 stays frozen at 300.
	status, body := post(t, srv.URL+"/addons/"+addonID.String()+"/versions", "")
	if status != http.StatusCreated {
		t.Fatalf("create addon v2: %d (%s)", status, body)
	}
	var v2 struct{ ID uuid.UUID }
	mustJSON(t, body, &v2)
	if status, b := do(t, http.MethodPatch, srv.URL+"/addons/"+addonID.String()+"/versions/"+v2.ID.String(),
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":600}],"compatible_plan_keys":["pro"]}`); status != http.StatusOK {
		t.Fatalf("update addon v2: %d (%s)", status, b)
	}
	post(t, srv.URL+"/addons/"+addonID.String()+"/versions/"+v2.ID.String()+"/publish", "")

	_, v1body := get(t, srv.URL+"/addon-versions/"+v1ID.String())
	if !strings.Contains(v1body, `"amount_minor":300`) {
		t.Fatalf("addon v1 snapshot mutated: %s", v1body)
	}
}

func TestAddonExposedViaCatalogReaderWithDeltas(t *testing.T) {
	deps, _ := newDeps(t)
	mod := catalog.New(deps)
	srv := adminServer(mod.Handler())
	defer srv.Close()

	_, v1ID := createPublishedAddon(t, srv.URL, "boost", []string{"pro", "enterprise"})

	info, err := mod.Port().GetAddonVersion(context.Background(), v1ID)
	if err != nil {
		t.Fatalf("GetAddonVersion: %v", err)
	}
	if info.AddonKey != "boost" || info.Status != "published" || !info.QuantityAllowed {
		t.Fatalf("addon snapshot = %+v", info)
	}
	if len(info.Deltas) != 1 || info.Deltas[0].FeatureKey != "max_projects" || info.Deltas[0].Amount != 10 {
		t.Fatalf("addon deltas = %+v", info.Deltas)
	}
	// The shared compatibility helper accepts and rejects correctly.
	if !info.CompatibleWith("pro") || !info.CompatibleWith("enterprise") {
		t.Fatalf("compatibility helper rejected a compatible plan: %v", info.CompatiblePlanKeys)
	}
	if info.CompatibleWith("starter") {
		t.Fatalf("compatibility helper accepted an incompatible plan")
	}
}
