//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant"
)

// TestEntitlementsResolutionLiveThroughWiredApp is the live boot check for T-022:
// it drives the fully wired application over HTTP against a real Postgres —
// register → tenant → plan (with a feature grant) → publish → subscribe → attach
// an addon carrying a limit delta → GET /entitlements — and asserts the
// "10 + addon(×1) ⇒ 20" resolution, then re-attaches at quantity 2 and asserts
// re-materialization to 30.
func TestEntitlementsResolutionLiveThroughWiredApp(t *testing.T) {
	a := testApp(t)
	srv := httptest.NewServer(a.handler)
	defer srv.Close()
	ctx := context.Background()

	// A user registers and logs in for a Bearer token.
	if _, err := http.Post(srv.URL+"/api/v1/auth/register", "application/json",
		strings.NewReader(`{"email":"ops@example.com","password":"ops-password-xyz"}`)); err != nil {
		t.Fatalf("register: %v", err)
	}
	token := login(t, srv.URL, "ops@example.com", "ops-password-xyz")
	tenantID := createTenant(t, srv.URL, "acme")

	// The operator registers a "seats" limit feature — features are data, so this
	// is a plain insert (no deploy). The registry admin API is out of T-022's HTTP
	// scope, so we insert directly, exactly as a seed/migration would.
	if _, err := a.pool.Exec(ctx,
		`INSERT INTO entitlements.features (id, key, type, default_value, description, limit_behavior, reset_period, metadata, active, created_at, updated_at)
		 VALUES ($1,'seats','limit','0'::jsonb,'seats','hard','billing_cycle','{}'::jsonb,true, now(), now())`,
		uuid.New()); err != nil {
		t.Fatalf("seed feature: %v", err)
	}

	// A plan with a base grant of 10 seats, published.
	planID, draftV := createPlan(t, srv.URL, token, "pro", "Pro")
	patchJSON(t, srv.URL+"/api/v1/catalog/plans/"+planID+"/versions/"+draftV, token,
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":1000}],"trial":{"enabled":false},"grace_days":0,"feature_grants":{"seats":10}}`)
	planVersionID := publish(t, srv.URL+"/api/v1/catalog/plans/"+planID+"/versions/"+draftV+"/publish", token)

	// An addon that adds 10 seats per unit, compatible with "pro", published.
	addonID, addonDraftV := createAddon(t, srv.URL, token, "extra-seats", "Extra Seats")
	patchJSON(t, srv.URL+"/api/v1/catalog/addons/"+addonID+"/versions/"+addonDraftV, token,
		`{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":500}],"quantity_allowed":true,"compatible_plan_keys":["pro"],"deltas":[{"feature_key":"seats","kind":"limit_delta","amount":10}]}`)
	addonVersionID := publish(t, srv.URL+"/api/v1/catalog/addons/"+addonID+"/versions/"+addonDraftV+"/publish", token)

	// The tenant subscribes to the plan, then attaches one unit of the addon.
	if status, body := tenantPost(t, srv.URL+"/api/v1/subscription", token, tenantID, `{"plan_version_id":"`+planVersionID+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatalf("subscribe = %d (%s)", status, body)
	}
	if status, body := tenantPost(t, srv.URL+"/api/v1/subscription/addons", token, tenantID, `{"addon_version_id":"`+addonVersionID+`","quantity":1}`); status != http.StatusOK {
		t.Fatalf("attach addon = %d (%s)", status, body)
	}

	// GET /entitlements resolves the whole set in one call: 10 (plan) + 10 (addon ×1) = 20.
	status, body := tenantGet(t, srv.URL+"/api/v1/entitlements", token, tenantID)
	t.Logf("GET /entitlements (x1) -> %d %s", status, body)
	if status != http.StatusOK {
		t.Fatalf("GET entitlements = %d (%s)", status, body)
	}
	assertSeats(t, body, 20, "addon")

	// GET /entitlements/{key} returns the single resolved value.
	skStatus, skBody := tenantGet(t, srv.URL+"/api/v1/entitlements/seats", token, tenantID)
	t.Logf("GET /entitlements/seats -> %d %s", skStatus, skBody)
	if skStatus != http.StatusOK || !strings.Contains(skBody, `"value":20`) {
		t.Fatalf("GET entitlements/seats = %d (%s), want value 20", skStatus, skBody)
	}

	// Draining the outbox materializes the set and publishes EntitlementsSummaryChanged.
	drainRelay(t, a)
	assertMaterializedSeats(t, a, tenantID, 20)
	assertSummaryEmitted(t, a, tenantID)

	// Raising the addon quantity to 2 re-materializes: 10 + 10×2 = 30.
	if status, body := tenantPost(t, srv.URL+"/api/v1/subscription/addons", token, tenantID, `{"addon_version_id":"`+addonVersionID+`","quantity":2}`); status != http.StatusOK {
		t.Fatalf("re-attach x2 = %d (%s)", status, body)
	}
	status, body = tenantGet(t, srv.URL+"/api/v1/entitlements", token, tenantID)
	t.Logf("GET /entitlements (x2) -> %d %s", status, body)
	assertSeats(t, body, 30, "addon")
	drainRelay(t, a)
	assertMaterializedSeats(t, a, tenantID, 30)
}

// --- flow helpers ---

func login(t *testing.T, baseURL, email, password string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json",
		strings.NewReader(`{"email":"`+email+`","password":"`+password+`"}`))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.AccessToken == "" {
		t.Fatalf("login token: %v (%s)", err, b)
	}
	return out.AccessToken
}

func createPlan(t *testing.T, baseURL, token, key, name string) (planID, versionID string) {
	t.Helper()
	status, body := authPost(t, baseURL+"/api/v1/catalog/plans", token, `{"key":"`+key+`","name":"`+name+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create plan = %d (%s)", status, body)
	}
	var out struct {
		Plan    struct{ ID string } `json:"plan"`
		Version struct{ ID string } `json:"version"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode plan: %v (%s)", err, body)
	}
	return out.Plan.ID, out.Version.ID
}

func createAddon(t *testing.T, baseURL, token, key, name string) (addonID, versionID string) {
	t.Helper()
	status, body := authPost(t, baseURL+"/api/v1/catalog/addons", token, `{"key":"`+key+`","name":"`+name+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create addon = %d (%s)", status, body)
	}
	var out struct {
		Addon   struct{ ID string } `json:"addon"`
		Version struct{ ID string } `json:"version"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode addon: %v (%s)", err, body)
	}
	return out.Addon.ID, out.Version.ID
}

func publish(t *testing.T, url, token string) string {
	t.Helper()
	status, body := authPost(t, url, token, ``)
	if status != http.StatusOK {
		t.Fatalf("publish = %d (%s)", status, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode publish: %v (%s)", err, body)
	}
	return out.ID
}

func assertSeats(t *testing.T, body string, want int, wantSource string) {
	t.Helper()
	var out struct {
		Entitlements map[string]struct {
			Value  float64 `json:"value"`
			Source string  `json:"source"`
		} `json:"entitlements"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode entitlements: %v (%s)", err, body)
	}
	seats, ok := out.Entitlements["seats"]
	if !ok {
		t.Fatalf("seats absent from entitlements: %s", body)
	}
	if int(seats.Value) != want || seats.Source != wantSource {
		t.Fatalf("seats = %v/%s, want %d/%s", seats.Value, seats.Source, want, wantSource)
	}
}

func drainRelay(t *testing.T, a *application) {
	t.Helper()
	for i := 0; i < 20; i++ {
		n, err := a.relay.ProcessBatch(context.Background())
		if err != nil {
			t.Fatalf("relay: %v", err)
		}
		if n == 0 {
			return
		}
	}
}

func assertMaterializedSeats(t *testing.T, a *application, tenantID uuid.UUID, want int64) {
	t.Helper()
	var raw []byte
	if err := a.pool.QueryRow(context.Background(),
		`SELECT value FROM entitlements.effective_entitlements WHERE tenant_id = $1 AND feature_key = 'seats'`, tenantID).Scan(&raw); err != nil {
		t.Fatalf("read materialized seats: %v", err)
	}
	var got int64
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal seats: %v", err)
	}
	if got != want {
		t.Fatalf("materialized seats = %d, want %d", got, want)
	}
}

func assertSummaryEmitted(t *testing.T, a *application, tenantID uuid.UUID) {
	t.Helper()
	var count int
	if err := a.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'entitlements.summary_changed'`, tenantID).Scan(&count); err != nil {
		t.Fatalf("count summary events: %v", err)
	}
	if count < 1 {
		t.Fatalf("no EntitlementsSummaryChanged event emitted")
	}
}

// --- request helpers ---

func authPost(t *testing.T, url, token, body string) (int, string) {
	t.Helper()
	return authedReq(t, http.MethodPost, url, token, "", body)
}

func patchJSON(t *testing.T, url, token, body string) {
	t.Helper()
	if status, resp := authedReq(t, http.MethodPatch, url, token, "", body); status != http.StatusOK {
		t.Fatalf("PATCH %s = %d (%s)", url, status, resp)
	}
}

func tenantPost(t *testing.T, url, token string, tenantID uuid.UUID, body string) (int, string) {
	t.Helper()
	return authedReq(t, http.MethodPost, url, token, tenantID.String(), body)
}

func tenantGet(t *testing.T, url, token string, tenantID uuid.UUID) (int, string) {
	t.Helper()
	return authedReq(t, http.MethodGet, url, token, tenantID.String(), "")
}

func authedReq(t *testing.T, method, url, token, tenantID, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenantID != "" {
		req.Header.Set(tenant.HeaderTenantID, tenantID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
