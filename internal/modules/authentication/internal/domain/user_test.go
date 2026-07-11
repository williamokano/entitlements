package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

func TestNormalizeEmailLowercasesAndValidates(t *testing.T) {
	got, err := NormalizeEmail("  User@Example.COM ")
	if err != nil || got != "user@example.com" {
		t.Fatalf("NormalizeEmail = (%q, %v), want (user@example.com, nil)", got, err)
	}

	for _, bad := range []string{"", "not-an-email", "a@b@c.com", "no-at-sign.com", "spaces in@x.com"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Errorf("NormalizeEmail(%q) = nil error, want validation error", bad)
		} else if apperr.KindOf(err) != apperr.KindValidation {
			t.Errorf("NormalizeEmail(%q) kind = %v, want validation", bad, apperr.KindOf(err))
		}
	}
}

func TestNewUserIsActiveWithNormalizedEmail(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u, err := NewUser(uuid.New(), "Alice@Example.com", now)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email = %q, want normalized", u.Email)
	}
	if !u.Active() || u.Status != StatusActive {
		t.Fatalf("status = %s, want active", u.Status)
	}
	if u.CreatedAt != now || u.UpdatedAt != now {
		t.Fatalf("timestamps not set from now")
	}
}
