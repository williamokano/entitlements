package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

func TestGETRequestsBypassIdempotency(t *testing.T) {
	// A nil pool is safe: a GET must bypass before any database access.
	var calls int
	h := httpx.Idempotency(nil, clock.System, time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/things", nil)
	req.Header.Set(httpx.HeaderIdempotencyKey, "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (GET bypasses idempotency)", calls)
	}
}
