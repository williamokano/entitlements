//go:build integration

package httpx_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/httpx"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

// withTenant injects a tenant into the request context (the real tenant
// middleware arrives in T-011).
func withTenant(tenant uuid.UUID, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(authctx.WithTenantID(r.Context(), tenant)))
	})
}

// countingHandler responds 201 with a unique body per call and counts calls.
func countingHandler(calls *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"call":`+itoa(n)+`}`)
	})
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func post(t *testing.T, url, key string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if key != "" {
		req.Header.Set(httpx.HeaderIdempotencyKey, key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func serve(t *testing.T, pool *pgxpool.Pool, clk clock.Clock, tenant uuid.UUID, h http.Handler) *httptest.Server {
	t.Helper()
	wrapped := withTenant(tenant, httpx.Idempotency(pool, clk, time.Hour)(h))
	srv := httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)
	return srv
}

func TestDuplicatePOSTReturnsStoredResponseHandlerRunsOnce(t *testing.T) {
	pool := testkit.Postgres(t)
	var calls atomic.Int64
	srv := serve(t, pool, clock.System, uuid.New(), countingHandler(&calls))

	status1, body1 := post(t, srv.URL+"/things", "key-1")
	status2, body2 := post(t, srv.URL+"/things", "key-1")

	if status1 != http.StatusCreated || status2 != http.StatusCreated {
		t.Fatalf("statuses = %d/%d, want 201/201", status1, status2)
	}
	if body1 != body2 {
		t.Fatalf("bodies differ: %q vs %q (replay should be identical)", body1, body2)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestConcurrentDuplicateReturns409(t *testing.T) {
	pool := testkit.Postgres(t)

	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		close(started)
		<-release
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	srv := serve(t, pool, clock.System, uuid.New(), h)

	// Request A enters the handler and blocks (its key is reserved).
	type result struct {
		status int
	}
	aCh := make(chan result, 1)
	go func() {
		s, _ := post(t, srv.URL+"/things", "dup")
		aCh <- result{status: s}
	}()

	<-started

	// Request B, same key, arrives while A is in flight → 409.
	statusB, _ := post(t, srv.URL+"/things", "dup")
	if statusB != http.StatusConflict {
		t.Fatalf("concurrent duplicate status = %d, want 409", statusB)
	}

	close(release)
	a := <-aCh
	if a.status != http.StatusCreated {
		t.Fatalf("original request status = %d, want 201", a.status)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestDifferentKeyExecutesHandlerAgain(t *testing.T) {
	pool := testkit.Postgres(t)
	var calls atomic.Int64
	srv := serve(t, pool, clock.System, uuid.New(), countingHandler(&calls))

	post(t, srv.URL+"/things", "key-a")
	post(t, srv.URL+"/things", "key-b")

	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2 (distinct keys)", calls.Load())
	}
}

func TestKeyScopedByTenantAndRoute(t *testing.T) {
	pool := testkit.Postgres(t)
	var calls atomic.Int64
	handler := countingHandler(&calls)

	// Same key, two different tenants → both execute.
	srvT1 := serve(t, pool, clock.System, uuid.New(), handler)
	srvT2 := serve(t, pool, clock.System, uuid.New(), handler)
	post(t, srvT1.URL+"/things", "shared")
	post(t, srvT2.URL+"/things", "shared")
	if calls.Load() != 2 {
		t.Fatalf("after two tenants: calls = %d, want 2", calls.Load())
	}

	// Same key and tenant, different route → both execute.
	tenant := uuid.New()
	srv := serve(t, pool, clock.System, tenant, handler)
	post(t, srv.URL+"/things", "sk")
	post(t, srv.URL+"/widgets", "sk")
	if calls.Load() != 4 {
		t.Fatalf("after two routes: calls = %d, want 4", calls.Load())
	}
}

func TestExpiredKeyReexecutes(t *testing.T) {
	pool := testkit.Postgres(t)
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	var calls atomic.Int64
	srv := serve(t, pool, clk, uuid.New(), countingHandler(&calls))

	post(t, srv.URL+"/things", "exp")
	// A replay before the TTL elapses does not re-execute.
	post(t, srv.URL+"/things", "exp")
	if calls.Load() != 1 {
		t.Fatalf("before TTL: calls = %d, want 1", calls.Load())
	}

	// Past the TTL (serve uses a 1h TTL), the key is reusable.
	clk.Advance(2 * time.Hour)
	post(t, srv.URL+"/things", "exp")
	if calls.Load() != 2 {
		t.Fatalf("after TTL: calls = %d, want 2", calls.Load())
	}
}
