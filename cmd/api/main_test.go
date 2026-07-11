package main

import "testing"

func TestNewLogger(t *testing.T) {
	// Known and unknown levels both yield a usable logger (unknown falls back
	// to info).
	for _, level := range []string{"debug", "info", "warn", "error", "nonsense", ""} {
		if got := newLogger(level); got == nil {
			t.Fatalf("newLogger(%q) = nil", level)
		}
	}
}

// The /healthz + /readyz probes are exercised end-to-end against the fully
// wired composition root in TestCompositionRootBootsAndServesHealthz
// (app_integration_test.go), which supersedes the earlier unit-level health
// check now that the handler requires a database.
