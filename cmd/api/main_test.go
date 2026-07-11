package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	srv := httptest.NewServer(newMux())
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
		})
	}
}

func TestPortDefault(t *testing.T) {
	t.Setenv("PORT", "")
	if got := port(); got != "8080" {
		t.Fatalf("port() = %q, want 8080", got)
	}
	t.Setenv("PORT", "9999")
	if got := port(); got != "9999" {
		t.Fatalf("port() = %q, want 9999", got)
	}
}
