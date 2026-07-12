//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant"
	"github.com/williamokano/entitlements/internal/platform/config"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

// createTenant provisions an active tenant through the tenant-admin API (which
// is exempt from tenant resolution) and returns its ID. Tenant-scoped module
// requests must carry this ID in the X-Tenant-ID header.
func createTenant(t *testing.T, baseURL, slug string) uuid.UUID {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/v1/tenants", "application/json",
		strings.NewReader(`{"slug":"`+slug+`","name":"`+slug+`"}`))
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tenant: status = %d, want 201 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode tenant: %v (%s)", err, body)
	}
	return out.ID
}

// postJSON sends a JSON POST scoped to the given tenant via the X-Tenant-ID
// header, returning the response for the caller to inspect.
func postJSON(t *testing.T, url string, tenantID uuid.UUID, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(tenant.HeaderTenantID, tenantID.String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func testApp(t *testing.T) *application {
	t.Helper()
	pool := testkit.Postgres(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a, err := buildApplication(config.Config{Environment: config.EnvDevelopment}, pool, logger)
	if err != nil {
		t.Fatalf("buildApplication: %v", err)
	}
	return a
}

func TestAPIKeyManagementAndMachineAuthE2E(t *testing.T) {
	a := testApp(t)
	srv := httptest.NewServer(a.handler)
	defer srv.Close()

	// A user registers and logs in to get an access token.
	if _, err := http.Post(srv.URL+"/api/v1/auth/register", "application/json",
		strings.NewReader(`{"email":"owner@example.com","password":"owner-password-x"}`)); err != nil {
		t.Fatalf("register: %v", err)
	}
	loginResp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json",
		strings.NewReader(`{"email":"owner@example.com","password":"owner-password-x"}`))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	lb, _ := io.ReadAll(loginResp.Body)
	_ = loginResp.Body.Close()
	var login struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(lb, &login)

	tenantID := createTenant(t, srv.URL, "keyco")

	// Managing keys without a credential is rejected by RequireAuth (the request
	// carries a tenant so it clears tenant resolution and reaches the guard).
	anonReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/api-keys", strings.NewReader(`{"name":"x"}`))
	anonReq.Header.Set("Content-Type", "application/json")
	anonReq.Header.Set(tenant.HeaderTenantID, tenantID.String())
	anon, err := http.DefaultClient.Do(anonReq)
	if err != nil {
		t.Fatalf("anon key create: %v", err)
	}
	_ = anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous key create = %d, want 401", anon.StatusCode)
	}

	// The user creates a key for their tenant (Bearer JWT + X-Tenant-ID).
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/api-keys",
		strings.NewReader(`{"name":"ci","scopes":["things:write"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)
	req.Header.Set(tenant.HeaderTenantID, tenantID.String())
	createResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	cb, _ := io.ReadAll(createResp.Body)
	_ = createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create key = %d (%s), want 201", createResp.StatusCode, cb)
	}
	var key struct {
		ID     uuid.UUID `json:"id"`
		APIKey string    `json:"api_key"`
	}
	if err := json.Unmarshal(cb, &key); err != nil {
		t.Fatalf("decode key: %v", err)
	}

	// The API key authenticates a tenant-scoped business route WITHOUT an
	// X-Tenant-ID header — the tenant is derived from the key.
	thingReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/example/things",
		strings.NewReader(`{"name":"via-key"}`))
	thingReq.Header.Set("Content-Type", "application/json")
	thingReq.Header.Set("Authorization", "ApiKey "+key.APIKey)
	thingResp, err := http.DefaultClient.Do(thingReq)
	if err != nil {
		t.Fatalf("machine call: %v", err)
	}
	_ = thingResp.Body.Close()
	if thingResp.StatusCode != http.StatusCreated {
		t.Fatalf("machine-authenticated thing create = %d, want 201 (tenant from key)", thingResp.StatusCode)
	}

	// Revoke the key; it is then rejected.
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/api-keys/"+key.ID.String(), nil)
	delReq.Header.Set("Authorization", "Bearer "+login.AccessToken)
	delReq.Header.Set(tenant.HeaderTenantID, tenantID.String())
	delResp, _ := http.DefaultClient.Do(delReq)
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", delResp.StatusCode)
	}

	revokedReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/example/things",
		strings.NewReader(`{"name":"nope"}`))
	revokedReq.Header.Set("Content-Type", "application/json")
	revokedReq.Header.Set("Authorization", "ApiKey "+key.APIKey)
	revokedResp, _ := http.DefaultClient.Do(revokedReq)
	_ = revokedResp.Body.Close()
	if revokedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked-key call = %d, want 401", revokedResp.StatusCode)
	}
}

func TestTenantProvisioningSeedsSystemRolesThroughOutbox(t *testing.T) {
	a := testApp(t)
	srv := httptest.NewServer(a.handler)
	defer srv.Close()
	ctx := context.Background()

	tenantID := createTenant(t, srv.URL, "rbacco")

	// The tenant.created event drives the authorization seed hook via the relay.
	if _, err := a.relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay: %v", err)
	}

	var count int
	if err := a.pool.QueryRow(ctx,
		`SELECT count(*) FROM authz.roles WHERE tenant_id = $1 AND system AND name IN ('owner','admin','member')`,
		tenantID).Scan(&count); err != nil {
		t.Fatalf("query roles: %v", err)
	}
	if count != 3 {
		t.Fatalf("seeded system roles = %d, want 3 (owner/admin/member)", count)
	}
}

func TestCompositionRootBootsAndServesHealthz(t *testing.T) {
	a := testApp(t)
	srv := httptest.NewServer(a.handler)
	defer srv.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, resp.StatusCode)
		}
		if resp.Header.Get("X-Request-Id") == "" {
			t.Fatalf("GET %s: missing X-Request-Id (middleware chain not wired)", path)
		}
	}
}

func TestExampleModuleEndpointPersistsAndResponds(t *testing.T) {
	a := testApp(t)
	srv := httptest.NewServer(a.handler)
	defer srv.Close()

	tenantID := createTenant(t, srv.URL, "widgetco")

	resp := postJSON(t, srv.URL+"/api/v1/example/things", tenantID, `{"name":"widget"}`)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", resp.StatusCode, body)
	}
	var created struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if created.Name != "widget" || created.ID == uuid.Nil {
		t.Fatalf("response = %+v, want name=widget and a non-nil id", created)
	}

	// A tenant-scoped route without a tenant is rejected: the example module is
	// behind the resolution middleware.
	unscoped, err := http.Post(srv.URL+"/api/v1/example/things", "application/json",
		strings.NewReader(`{"name":"orphan"}`))
	if err != nil {
		t.Fatalf("POST unscoped: %v", err)
	}
	_ = unscoped.Body.Close()
	if unscoped.StatusCode != http.StatusBadRequest {
		t.Fatalf("unscoped POST status = %d, want 400 (no tenant)", unscoped.StatusCode)
	}

	// The row is persisted and readable back through the module (scoped).
	getReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/example/things/"+created.ID.String(), nil)
	getReq.Header.Set(tenant.HeaderTenantID, tenantID.String())
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	getBody, _ := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK || !strings.Contains(string(getBody), "widget") {
		t.Fatalf("GET thing = %d %s, want 200 containing widget", getResp.StatusCode, getBody)
	}
}

func TestExampleModuleEventFlowsThroughOutboxToConsumer(t *testing.T) {
	a := testApp(t)
	srv := httptest.NewServer(a.handler)
	defer srv.Close()
	ctx := context.Background()

	tenantID := createTenant(t, srv.URL, "eventco")

	// Provisioning the tenant emits its own tenant.created event; drain it so the
	// batch below observes only the example module's event.
	if _, err := a.relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("drain tenant.created: %v", err)
	}

	resp := postJSON(t, srv.URL+"/api/v1/example/things", tenantID, `{"name":"eventful"}`)
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// The relay delivers the outbox event to the module's consumer, which marks
	// the thing processed exactly once.
	published, err := a.relay.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("relay ProcessBatch: %v", err)
	}
	if published != 1 {
		t.Fatalf("relay published %d events, want 1", published)
	}

	// A second pass finds nothing new, and the idempotent consumer keeps the
	// count at exactly one.
	if _, err := a.relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("relay ProcessBatch #2: %v", err)
	}

	var processCount int
	if err := a.pool.QueryRow(ctx,
		`SELECT process_count FROM example.things WHERE id = $1`, created.ID).Scan(&processCount); err != nil {
		t.Fatalf("read process_count: %v", err)
	}
	if processCount != 1 {
		t.Fatalf("process_count = %d, want exactly 1 (outbox → consumer)", processCount)
	}
}
