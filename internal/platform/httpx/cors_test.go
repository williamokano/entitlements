package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func corsHandler(origins []string) http.Handler {
	return CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
}

func TestCORSPreflightShortCircuitsWithHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set(headerACRequestMethod, "POST")

	corsHandler([]string{"http://localhost:3000"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get(headerACAllowOrigin); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q, want the request origin", got)
	}
	if !containsHeaderToken(rec.Header().Get(headerACAllowHeaders), "X-Tenant-ID") ||
		!containsHeaderToken(rec.Header().Get(headerACAllowHeaders), "Authorization") ||
		!containsHeaderToken(rec.Header().Get(headerACAllowHeaders), HeaderIdempotencyKey) {
		t.Fatalf("allow-headers missing a required header: %q", rec.Header().Get(headerACAllowHeaders))
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("preflight should not reach the handler; body = %q", rec.Body.String())
	}
}

func TestCORSActualRequestGetsAllowOriginAndReachesHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	corsHandler([]string{"http://localhost:3000"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("actual request did not reach handler: %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(headerACAllowOrigin); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q, want the request origin", got)
	}
}

func TestCORSDisallowedOriginGetsNoHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://evil.example")

	corsHandler([]string{"http://localhost:3000"}).ServeHTTP(rec, req)

	if got := rec.Header().Get(headerACAllowOrigin); got != "" {
		t.Fatalf("allow-origin = %q, want empty for a disallowed origin", got)
	}
	// The request still reaches the handler; the browser (not the server) blocks
	// the response when the header is absent.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (server does not block; browser does)", rec.Code)
	}
}

func TestCORSWildcardAllowsAnyOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://anything.example")

	corsHandler([]string{"*"}).ServeHTTP(rec, req)

	if got := rec.Header().Get(headerACAllowOrigin); got != "*" {
		t.Fatalf("allow-origin = %q, want * for wildcard config", got)
	}
}

func containsHeaderToken(list, token string) bool {
	for _, part := range strings.Split(list, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}
