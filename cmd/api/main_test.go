package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	srv := httptest.NewServer(newHandler(newLogger("error")))
	defer srv.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want %d", path, resp.StatusCode, http.StatusOK)
			}
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("GET %s: content-type = %q, want application/json", path, got)
			}
			// The middleware chain must attach a request ID to every response.
			if resp.Header.Get("X-Request-Id") == "" {
				t.Fatalf("GET %s: missing X-Request-Id header", path)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "nonsense", ""} {
		if got := newLogger(level); got == nil {
			t.Fatalf("newLogger(%q) = nil", level)
		}
	}
}
