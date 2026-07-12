package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

func TestNewInvitationNormalizesEmailAndRequiresRole(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	inv, err := NewInvitation(uuid.New(), uuid.New(), uuid.New(), "  User@Example.COM ", "member", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("NewInvitation: %v", err)
	}
	if inv.Email != "user@example.com" {
		t.Fatalf("email = %q, want normalized", inv.Email)
	}
	if inv.Status != InvitePending {
		t.Fatalf("status = %s, want pending", inv.Status)
	}

	if _, err := NewInvitation(uuid.New(), uuid.New(), uuid.New(), "a@b.com", "  ", now, now.Add(time.Hour)); err == nil {
		t.Fatal("empty role accepted, want validation error")
	}
	if _, err := NewInvitation(uuid.New(), uuid.New(), uuid.New(), "not-an-email", "member", now, now.Add(time.Hour)); apperr.KindOf(err) != apperr.KindValidation {
		t.Fatalf("bad email kind = %v, want validation", apperr.KindOf(err))
	}
}

func TestInvitationAcceptable(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	inv := &Invitation{Status: InvitePending, ExpiresAt: now.Add(time.Hour)}

	if !inv.Acceptable(now) {
		t.Fatal("pending, unexpired invitation not acceptable")
	}
	if inv.Acceptable(now.Add(2 * time.Hour)) {
		t.Fatal("expired invitation still acceptable")
	}
	inv.Status = InviteAccepted
	if inv.Acceptable(now) {
		t.Fatal("already-accepted invitation acceptable again")
	}
}
