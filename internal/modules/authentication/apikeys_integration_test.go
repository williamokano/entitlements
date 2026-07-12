//go:build integration

package authentication_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/authctx"
)

// withTenantAndUser wraps a handler so it sees a resolved tenant and an
// authenticated user principal, standing in for the tenant-resolution + auth
// middleware the composition root applies to /api/v1/api-keys.
func withTenantAndUser(h http.Handler, tenantID uuid.UUID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithTenantID(r.Context(), tenantID)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: uuid.NewString()})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

type createdKey struct {
	ID     uuid.UUID `json:"id"`
	Prefix string    `json:"prefix"`
	APIKey string    `json:"api_key"`
	Scopes []string  `json:"scopes"`
}

func createKey(t *testing.T, url string, body string) createdKey {
	t.Helper()
	status, respBody := post(t, url, body)
	if status != http.StatusCreated {
		t.Fatalf("create key status = %d (%s), want 201", status, respBody)
	}
	var k createdKey
	if err := json.Unmarshal([]byte(respBody), &k); err != nil {
		t.Fatalf("decode key: %v (%s)", err, respBody)
	}
	return k
}

func TestAPIKeySecretShownOnceAndHashedAtRest(t *testing.T) {
	mod, _, deps := newModule(t)
	tenantID := uuid.New()
	admin := httptest.NewServer(withTenantAndUser(mod.APIKeysHandler(), tenantID))
	defer admin.Close()

	key := createKey(t, admin.URL, `{"name":"ci","scopes":["things:read"]}`)
	if key.APIKey == "" || !strings.Contains(key.APIKey, ".") {
		t.Fatalf("create response missing the one-time api_key: %+v", key)
	}

	// The raw secret is never stored; only an argon2id hash is.
	var secretHash string
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT secret_hash FROM authn.api_keys WHERE id = $1`, key.ID).Scan(&secretHash); err != nil {
		t.Fatalf("query hash: %v", err)
	}
	_, rawSecret, _ := splitKey(key.APIKey)
	if !strings.HasPrefix(secretHash, "$argon2id$") || strings.Contains(secretHash, rawSecret) {
		t.Fatalf("secret not hashed at rest: %q", secretHash)
	}

	// List never returns the secret (neither the raw secret nor a per-key
	// "api_key" field — note the wrapper object is "api_keys", which is fine).
	_, listBody := get(t, admin.URL)
	if strings.Contains(listBody, rawSecret) || strings.Contains(listBody, `"api_key":`) {
		t.Fatalf("list leaked the secret: %s", listBody)
	}
}

func TestAPIKeyAuthSetsMachinePrincipalWithScopes(t *testing.T) {
	mod, _, _ := newModule(t)
	tenantID := uuid.New()
	admin := httptest.NewServer(withTenantAndUser(mod.APIKeysHandler(), tenantID))
	defer admin.Close()

	key := createKey(t, admin.URL, `{"name":"svc","scopes":["things:read","things:write"]}`)

	// A probe behind the auth middleware reports the principal + tenant.
	var principal authctx.Principal
	var tid uuid.UUID
	probe := mod.AuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, _ = authctx.PrincipalFromContext(r.Context())
		tid, _ = authctx.TenantID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(probe)
	defer srv.Close()

	if status := getWithKey(t, srv.URL, key.APIKey); status != http.StatusOK {
		t.Fatalf("machine auth status = %d, want 200", status)
	}
	if principal.Kind != authctx.PrincipalMachine {
		t.Fatalf("principal kind = %s, want machine", principal.Kind)
	}
	if !principal.HasScope("things:write") || !principal.HasScope("things:read") {
		t.Fatalf("principal scopes = %v, want the key's scopes", principal.Scopes)
	}
	if tid != tenantID {
		t.Fatalf("tenant = %s, want %s (api key binds its tenant)", tid, tenantID)
	}
}

func TestRevokedAPIKeyRejected401(t *testing.T) {
	mod, _, _ := newModule(t)
	tenantID := uuid.New()
	admin := httptest.NewServer(withTenantAndUser(mod.APIKeysHandler(), tenantID))
	defer admin.Close()

	key := createKey(t, admin.URL, `{"name":"revoke-me","scopes":[]}`)

	probe := mod.AuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(probe)
	defer srv.Close()

	// Works before revocation.
	if status := getWithKey(t, srv.URL, key.APIKey); status != http.StatusOK {
		t.Fatalf("pre-revoke status = %d, want 200", status)
	}
	// Revoke, then it is rejected immediately.
	if status, _ := do(t, http.MethodDelete, admin.URL+"/"+key.ID.String(), ""); status != http.StatusNoContent {
		t.Fatalf("revoke status, want 204")
	}
	if status := getWithKey(t, srv.URL, key.APIKey); status != http.StatusUnauthorized {
		t.Fatalf("post-revoke status = %d, want 401", status)
	}
}

func TestAPIKeyLastUsedAtUpdatedOnUse(t *testing.T) {
	mod, _, deps := newModule(t)
	tenantID := uuid.New()
	admin := httptest.NewServer(withTenantAndUser(mod.APIKeysHandler(), tenantID))
	defer admin.Close()

	key := createKey(t, admin.URL, `{"name":"touch","scopes":[]}`)

	// Fresh key: last_used_at is null.
	var beforeNull bool
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT last_used_at IS NULL FROM authn.api_keys WHERE id = $1`, key.ID).Scan(&beforeNull); err != nil {
		t.Fatalf("query before: %v", err)
	}
	if !beforeNull {
		t.Fatal("last_used_at set before first use")
	}

	probe := mod.AuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	srv := httptest.NewServer(probe)
	defer srv.Close()
	_ = getWithKey(t, srv.URL, key.APIKey)

	var afterSet bool
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT last_used_at IS NOT NULL FROM authn.api_keys WHERE id = $1`, key.ID).Scan(&afterSet); err != nil {
		t.Fatalf("query after: %v", err)
	}
	if !afterSet {
		t.Fatal("last_used_at not updated after use")
	}
}

func TestAPIKeyCRUDIsTenantScoped(t *testing.T) {
	mod, _, _ := newModule(t)
	tenantA, tenantB := uuid.New(), uuid.New()
	adminA := httptest.NewServer(withTenantAndUser(mod.APIKeysHandler(), tenantA))
	defer adminA.Close()
	adminB := httptest.NewServer(withTenantAndUser(mod.APIKeysHandler(), tenantB))
	defer adminB.Close()

	key := createKey(t, adminA.URL, `{"name":"a-key","scopes":[]}`)

	// Tenant B cannot see A's key...
	_, listB := get(t, adminB.URL)
	if strings.Contains(listB, key.ID.String()) {
		t.Fatalf("tenant B sees tenant A's key: %s", listB)
	}
	// ...nor revoke it.
	if status, _ := do(t, http.MethodDelete, adminB.URL+"/"+key.ID.String(), ""); status != http.StatusNotFound {
		t.Fatalf("cross-tenant revoke status = %d, want 404", status)
	}
	// A still lists its own key.
	_, listA := get(t, adminA.URL)
	if !strings.Contains(listA, key.ID.String()) {
		t.Fatalf("tenant A lost its key: %s", listA)
	}
}

// --- helpers ---

func splitKey(full string) (prefix, secret string, ok bool) {
	prefix, secret, found := strings.Cut(full, ".")
	return prefix, secret, found
}

func get(t *testing.T, url string) (int, string) { return do(t, http.MethodGet, url, "") }

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func getWithKey(t *testing.T, url, apiKey string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "ApiKey "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
