package jobs_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func noop(context.Context) error { return nil }

func TestRegisterRejectsDuplicateJobNames(t *testing.T) {
	// Register touches only in-memory state, so a nil pool is fine here.
	r := jobs.NewRunner(nil, id.UUIDv7{}, clock.System, discardLogger(), time.Second)

	if err := r.Register("renew", time.Minute, noop); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("renew", time.Minute, noop); err == nil {
		t.Fatal("second Register of same name = nil, want duplicate error")
	}
	if err := r.Register("dunning", time.Minute, noop); err != nil {
		t.Fatalf("Register of distinct name: %v", err)
	}

	// Guard rails on bad input.
	if err := r.Register("", time.Minute, noop); err == nil {
		t.Error("empty name accepted, want error")
	}
	if err := r.Register("zero", 0, noop); err == nil {
		t.Error("non-positive interval accepted, want error")
	}
}
