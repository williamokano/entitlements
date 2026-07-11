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

func TestNewLogger(t *testing.T) {
	// Known and unknown levels both yield a usable logger (unknown falls back
	// to info).
	for _, level := range []string{"debug", "info", "warn", "error", "nonsense", ""} {
		if got := newLogger(level); got == nil {
			t.Fatalf("newLogger(%q) = nil", level)
		}
	}
}
