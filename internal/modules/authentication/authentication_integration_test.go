//go:build integration

package authentication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/authentication"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newDeps(t *testing.T) (app.Deps, *events.Bus) {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	clk := clock.System
	bus := events.NewBus()
	deps := app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        bus,
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
	return deps, bus
}

func newModule(t *testing.T, opts ...authentication.Option) (*authentication.Module, *events.Bus, app.Deps) {
	t.Helper()
	deps, bus := newDeps(t)
	mod, err := authentication.New(deps, opts...)
	if err != nil {
		t.Fatalf("authentication.New: %v", err)
	}
	return mod, bus, deps
}

func TestRegisterLoginRefreshLogoutFlow(t *testing.T) {
	mod, _, _ := newModule(t)
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	// Register.
	status, body := post(t, srv.URL+"/register", `{"email":"alice@example.com","password":"s3cret-pass-phrase"}`)
	if status != http.StatusCreated {
		t.Fatalf("register status = %d (%s), want 201", status, body)
	}

	// Login → tokens.
	status, body = post(t, srv.URL+"/login", `{"email":"alice@example.com","password":"s3cret-pass-phrase"}`)
	if status != http.StatusOK {
		t.Fatalf("login status = %d (%s), want 200", status, body)
	}
	tokens := decodeTokens(t, body)
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("login returned empty tokens: %+v", tokens)
	}
	// The access token verifies offline with the module's published verifier.
	ident, err := mod.Verifier().Verify(tokens.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if ident.Email != "alice@example.com" {
		t.Fatalf("verified identity email = %q, want alice@example.com", ident.Email)
	}

	// Refresh → new token pair; old refresh token is invalidated.
	status, body = post(t, srv.URL+"/refresh", `{"refresh_token":"`+tokens.RefreshToken+`"}`)
	if status != http.StatusOK {
		t.Fatalf("refresh status = %d (%s), want 200", status, body)
	}
	rotated := decodeTokens(t, body)
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh returned the same refresh token (no rotation)")
	}
	// The old refresh token no longer works (reuse detection).
	if status, _ := post(t, srv.URL+"/refresh", `{"refresh_token":"`+tokens.RefreshToken+`"}`); status != http.StatusUnauthorized {
		t.Fatalf("reused old refresh status = %d, want 401", status)
	}
	// Reuse must revoke the WHOLE family (committed, not rolled back): the
	// rotated token that was live before the reuse is now dead too.
	if status, _ := post(t, srv.URL+"/refresh", `{"refresh_token":"`+rotated.RefreshToken+`"}`); status != http.StatusUnauthorized {
		t.Fatalf("rotated token after family reuse-revocation status = %d, want 401", status)
	}

	// A fresh login starts a new family so the account still works after the
	// compromised family was burned.
	_, body = post(t, srv.URL+"/login", `{"email":"alice@example.com","password":"s3cret-pass-phrase"}`)
	fresh := decodeTokens(t, body)

	// Logout the fresh token; afterwards it cannot refresh.
	if status, _ := post(t, srv.URL+"/logout", `{"refresh_token":"`+fresh.RefreshToken+`"}`); status != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", status)
	}
	if status, _ := post(t, srv.URL+"/refresh", `{"refresh_token":"`+fresh.RefreshToken+`"}`); status != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401", status)
	}
}

func TestRegisterDuplicateEmailConflict409(t *testing.T) {
	mod, _, _ := newModule(t)
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"dup@example.com","password":"password-one-two"}`); status != http.StatusCreated {
		t.Fatalf("first register, want 201")
	}
	// Same email, different case — normalization makes it a duplicate.
	if status, _ := post(t, srv.URL+"/register", `{"email":"DUP@example.com","password":"password-three-four"}`); status != http.StatusConflict {
		t.Fatalf("duplicate register status, want 409")
	}
}

func TestLoginWrongPassword401AndLoginFailedEventEmitted(t *testing.T) {
	mod, bus, deps := newModule(t)
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	var mu sync.Mutex
	var failed int
	bus.Subscribe(ports.EventLoginFailed, func(_ context.Context, _ events.Event) error {
		mu.Lock()
		failed++
		mu.Unlock()
		return nil
	})
	relay := events.NewRelay(deps.Pool, bus, deps.Clock, deps.Logger, events.RelayConfig{})

	if status, _ := post(t, srv.URL+"/register", `{"email":"bob@example.com","password":"right-password-here"}`); status != http.StatusCreated {
		t.Fatal("register bob, want 201")
	}

	// Wrong password and unknown user produce the same 401 (no enumeration).
	wrongStatus, wrongBody := post(t, srv.URL+"/login", `{"email":"bob@example.com","password":"WRONG"}`)
	unknownStatus, unknownBody := post(t, srv.URL+"/login", `{"email":"ghost@example.com","password":"WRONG"}`)
	if wrongStatus != http.StatusUnauthorized || unknownStatus != http.StatusUnauthorized {
		t.Fatalf("wrong=%d unknown=%d, want both 401", wrongStatus, unknownStatus)
	}
	if wrongBody != unknownBody {
		t.Fatalf("error bodies differ (enumeration leak):\n wrong=%s\n unknown=%s", wrongBody, unknownBody)
	}

	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("relay: %v", err)
	}
	mu.Lock()
	got := failed
	mu.Unlock()
	if got < 2 {
		t.Fatalf("login.failed events delivered = %d, want >= 2", got)
	}
}

func TestRefreshTokensHashedAtRest(t *testing.T) {
	mod, _, deps := newModule(t)
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"carol@example.com","password":"another-strong-pass"}`); status != http.StatusCreated {
		t.Fatal("register carol, want 201")
	}
	_, body := post(t, srv.URL+"/login", `{"email":"carol@example.com","password":"another-strong-pass"}`)
	tokens := decodeTokens(t, body)

	// The raw refresh token value must be absent from the database; only its hash
	// is stored.
	var rawCount int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM authn.refresh_tokens WHERE token_hash = $1`, tokens.RefreshToken).Scan(&rawCount); err != nil {
		t.Fatalf("query by raw value: %v", err)
	}
	if rawCount != 0 {
		t.Fatal("raw refresh token value found in DB (should be hashed at rest)")
	}
	// The hash is present.
	hash := sha256Hex(tokens.RefreshToken)
	var hashCount int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM authn.refresh_tokens WHERE token_hash = $1`, hash).Scan(&hashCount); err != nil {
		t.Fatalf("query by hash: %v", err)
	}
	if hashCount != 1 {
		t.Fatalf("hashed token rows = %d, want 1", hashCount)
	}

	// Passwords are hashed too (argon2id), never stored in clear.
	var secret string
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT secret FROM authn.credentials WHERE type = 'password' LIMIT 1`).Scan(&secret); err != nil {
		t.Fatalf("query credential: %v", err)
	}
	if !strings.HasPrefix(secret, "$argon2id$") || strings.Contains(secret, "another-strong-pass") {
		t.Fatalf("password not stored as argon2id hash: %q", secret)
	}
}

func TestRateLimitHookInvokedOnLogin(t *testing.T) {
	rec := &recordingLimiter{}
	mod, _, _ := newModule(t, authentication.WithRateLimiter(rec))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"dan@example.com","password":"rate-limit-pass"}`); status != http.StatusCreated {
		t.Fatal("register dan, want 201")
	}
	_, _ = post(t, srv.URL+"/login", `{"email":"dan@example.com","password":"rate-limit-pass"}`)
	_, _ = post(t, srv.URL+"/login", `{"email":"dan@example.com","password":"WRONG"}`)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.keys) < 2 {
		t.Fatalf("rate limiter saw %d attempts, want >= 2", len(rec.keys))
	}
	if rec.keys[0] != "dan@example.com" {
		t.Fatalf("rate limiter key = %q, want dan@example.com", rec.keys[0])
	}
}

func TestRateLimiterBlocksAfterThreshold(t *testing.T) {
	// The default in-memory limiter blocks once the per-window threshold is hit.
	limiter := authentication.NewMemoryRateLimiter(clock.System, 2, time.Hour)
	mod, _, _ := newModule(t, authentication.WithRateLimiter(limiter))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"eve@example.com","password":"blockme-password"}`); status != http.StatusCreated {
		t.Fatal("register eve, want 201")
	}
	// 2 allowed, 3rd blocked (403) before credentials are even checked.
	_, _ = post(t, srv.URL+"/login", `{"email":"eve@example.com","password":"WRONG"}`)
	_, _ = post(t, srv.URL+"/login", `{"email":"eve@example.com","password":"WRONG"}`)
	if status, _ := post(t, srv.URL+"/login", `{"email":"eve@example.com","password":"blockme-password"}`); status != http.StatusForbidden {
		t.Fatalf("3rd attempt status = %d, want 403 (rate limited)", status)
	}
}

// --- helpers ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func decodeTokens(t *testing.T, body string) tokenResponse {
	t.Helper()
	var out tokenResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode tokens from %q: %v", body, err)
	}
	return out
}

// recordingLimiter is a ports.RateLimiter that records every key it sees.
type recordingLimiter struct {
	mu   sync.Mutex
	keys []string
}

func (r *recordingLimiter) Allow(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = append(r.keys, key)
	return nil
}

func sha256Hex(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
