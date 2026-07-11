package domain

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// memRefreshRepo is an in-memory RefreshRepository for unit-testing rotation and
// family-reuse logic without a database.
type memRefreshRepo struct {
	byHash map[string]*RefreshToken
	byID   map[uuid.UUID]*RefreshToken
}

func newMemRefreshRepo() *memRefreshRepo {
	return &memRefreshRepo{
		byHash: map[string]*RefreshToken{},
		byID:   map[uuid.UUID]*RefreshToken{},
	}
}

func (m *memRefreshRepo) Insert(_ context.Context, t *RefreshToken) error {
	cp := *t
	m.byHash[t.Hash] = &cp
	m.byID[t.ID] = &cp
	return nil
}

func (m *memRefreshRepo) FindByHash(_ context.Context, hash string) (*RefreshToken, error) {
	if t, ok := m.byHash[hash]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, apperr.NotFound("refresh token not found")
}

func (m *memRefreshRepo) MarkRotated(_ context.Context, id uuid.UUID) error {
	if t, ok := m.byID[id]; ok {
		t.Status = RefreshRotated
	}
	return nil
}

func (m *memRefreshRepo) RevokeFamily(_ context.Context, familyID uuid.UUID) error {
	for _, t := range m.byID {
		if t.FamilyID == familyID {
			t.Status = RefreshRevoked
		}
	}
	return nil
}

func (m *memRefreshRepo) RevokeUser(_ context.Context, userID uuid.UUID) error {
	for _, t := range m.byID {
		if t.UserID == userID {
			t.Status = RefreshRevoked
		}
	}
	return nil
}

func (m *memRefreshRepo) RevokeFamiliesExcept(_ context.Context, userID, keep uuid.UUID) error {
	for _, t := range m.byID {
		if t.UserID == userID && t.FamilyID != keep {
			t.Status = RefreshRevoked
		}
	}
	return nil
}

func (m *memRefreshRepo) ListSessions(_ context.Context, userID uuid.UUID) ([]Session, error) {
	var out []Session
	for _, t := range m.byID {
		if t.UserID == userID && t.Status == RefreshActive {
			out = append(out, Session{ID: t.FamilyID, IssuedAt: t.IssuedAt, ExpiresAt: t.ExpiresAt})
		}
	}
	return out, nil
}

func mustIssue(t *testing.T, svc *RefreshService, userID uuid.UUID, raw string) *RefreshToken {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok, err := svc.Issue(context.Background(), uuid.New(), userID, raw, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

func TestRefreshRotationInvalidatesPreviousToken(t *testing.T) {
	svc := NewRefreshService(newMemRefreshRepo())
	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mustIssue(t, svc, userID, "rawA")

	// Rotate A -> B.
	res, err := svc.Rotate(context.Background(), "rawA", uuid.New(), "rawB", now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Rotate A->B: %v", err)
	}
	if res.Reused || res.Successor == nil || res.Successor.Status != RefreshActive {
		t.Fatalf("Rotate A->B result = %+v, want an active successor", res)
	}

	// The previous token A can no longer be rotated (it is now rotated, so a
	// second attempt is detected as reuse).
	reuse, err := svc.Rotate(context.Background(), "rawA", uuid.New(), "rawX", now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("re-rotating previous token errored = %v, want reuse result", err)
	}
	if !reuse.Reused {
		t.Fatal("re-rotating the previous token was not detected as reuse")
	}
}

func TestRefreshReuseRevokesEntireFamily(t *testing.T) {
	svc := NewRefreshService(newMemRefreshRepo())
	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mustIssue(t, svc, userID, "rawA")
	// A -> B (B is the live token).
	if _, err := svc.Rotate(context.Background(), "rawA", uuid.New(), "rawB", now, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("Rotate A->B: %v", err)
	}

	// Reuse of the already-rotated A must revoke the whole family and report
	// Reused (with a nil error, so the caller commits the revocation).
	reuse, err := svc.Rotate(context.Background(), "rawA", uuid.New(), "rawC", now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("reuse Rotate errored = %v, want reuse result", err)
	}
	if !reuse.Reused {
		t.Fatal("reuse of rotated token not detected")
	}

	// The live descendant B is now dead too: rotating it is also detected as
	// reuse (its status was revoked with the family).
	dead, err := svc.Rotate(context.Background(), "rawB", uuid.New(), "rawD", now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("rotating revoked descendant errored = %v, want reuse result", err)
	}
	if !dead.Reused {
		t.Fatal("descendant B still usable after family revocation")
	}
}

func TestRevokeIsIdempotentForUnknownToken(t *testing.T) {
	svc := NewRefreshService(newMemRefreshRepo())
	if err := svc.Revoke(context.Background(), "never-issued"); err != nil {
		t.Fatalf("Revoke unknown token = %v, want nil (idempotent)", err)
	}
}
