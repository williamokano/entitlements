package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// RefreshStatus is a refresh token's state within its family.
type RefreshStatus string

// Refresh token states.
const (
	// RefreshActive can be presented once to rotate into a successor.
	RefreshActive RefreshStatus = "active"
	// RefreshRotated has already produced a successor; presenting it again is
	// reuse and revokes the whole family.
	RefreshRotated RefreshStatus = "rotated"
	// RefreshRevoked was invalidated (logout or family revocation).
	RefreshRevoked RefreshStatus = "revoked"
)

// RefreshToken is one link in a per-login token family. Only its hash is stored;
// the raw value exists just long enough to be returned to the client.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	ParentID  uuid.NullUUID
	Hash      string
	Status    RefreshStatus
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// HashToken hashes a raw opaque token for storage and lookup. Opaque tokens
// (refresh tokens, verification/reset tokens) are high-entropy random strings,
// so a fast SHA-256 is appropriate — argon2 is only for low-entropy passwords.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// HashRefreshToken hashes a raw refresh token. It is HashToken specialized for
// refresh tokens (kept for call-site clarity).
func HashRefreshToken(raw string) string { return HashToken(raw) }

// Session is an active refresh-token family: one login on one device. The head
// (most recent, still-active) token represents the session.
type Session struct {
	ID        uuid.UUID // the family id
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// RefreshRepository persists refresh tokens. Implementations route through the
// ambient UnitOfWork transaction so rotation and family revocation are atomic.
type RefreshRepository interface {
	Insert(ctx context.Context, t *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	MarkRotated(ctx context.Context, id uuid.UUID) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeUser(ctx context.Context, userID uuid.UUID) error
	// RevokeFamiliesExcept revokes every active family of the user except keep
	// (used by "log out other sessions").
	RevokeFamiliesExcept(ctx context.Context, userID, keep uuid.UUID) error
	// ListSessions returns the user's active sessions (one per live family).
	ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error)
}

// RefreshService issues and rotates refresh tokens over a RefreshRepository. It
// is pure domain logic: unit-testable against an in-memory repository with no
// database.
type RefreshService struct {
	repo RefreshRepository
}

// NewRefreshService builds a RefreshService.
func NewRefreshService(repo RefreshRepository) *RefreshService {
	return &RefreshService{repo: repo}
}

// Issue stores a brand-new refresh token that starts its own family (its family
// id is its own id).
func (s *RefreshService) Issue(ctx context.Context, id, userID uuid.UUID, raw string, issuedAt, expiresAt time.Time) (*RefreshToken, error) {
	t := &RefreshToken{
		ID:        id,
		UserID:    userID,
		FamilyID:  id,
		Hash:      HashRefreshToken(raw),
		Status:    RefreshActive,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}
	if err := s.repo.Insert(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// RotationResult is the outcome of a rotation attempt.
type RotationResult struct {
	// Successor is the newly issued token on a successful rotation; it is nil
	// when Reused is true.
	Successor *RefreshToken
	// Reused reports that an already-used (or revoked) token was presented and
	// the whole family has been revoked. The caller must reject the request —
	// but as a committed side effect, not a rolled-back error, so the revocation
	// survives. A returned error, by contrast, rolls the transaction back.
	Reused bool
}

// Rotate validates a presented refresh token and issues its successor within
// the same family. Presenting a token that is not active — because it was
// already rotated (reuse), revoked, or logged out — revokes the entire family
// so a stolen token cannot outlive its own first legitimate use, and reports
// Reused=true with a nil error so the caller commits the revocation.
//
// A missing or expired token is an unauthorized error (nothing to persist, so a
// rollback is harmless).
func (s *RefreshService) Rotate(ctx context.Context, presentedRaw string, newID uuid.UUID, newRaw string, now, expiresAt time.Time) (RotationResult, error) {
	current, err := s.repo.FindByHash(ctx, HashRefreshToken(presentedRaw))
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			return RotationResult{}, apperr.Unauthorized("refresh token is no longer valid")
		}
		return RotationResult{}, err
	}

	if current.Status != RefreshActive {
		// Reuse of an already-used or revoked token: kill the family so no
		// descendant (including one an attacker may be holding) still works.
		if err := s.repo.RevokeFamily(ctx, current.FamilyID); err != nil {
			return RotationResult{}, err
		}
		return RotationResult{Reused: true}, nil
	}
	if now.After(current.ExpiresAt) {
		return RotationResult{}, apperr.Unauthorized("refresh token is no longer valid")
	}

	if err := s.repo.MarkRotated(ctx, current.ID); err != nil {
		return RotationResult{}, err
	}
	successor := &RefreshToken{
		ID:        newID,
		UserID:    current.UserID,
		FamilyID:  current.FamilyID,
		ParentID:  uuid.NullUUID{UUID: current.ID, Valid: true},
		Hash:      HashRefreshToken(newRaw),
		Status:    RefreshActive,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}
	if err := s.repo.Insert(ctx, successor); err != nil {
		return RotationResult{}, err
	}
	return RotationResult{Successor: successor}, nil
}

// Revoke invalidates a presented refresh token's family (logout). A token that
// cannot be found is treated as already gone (no error) so logout is idempotent.
func (s *RefreshService) Revoke(ctx context.Context, presentedRaw string) error {
	current, err := s.repo.FindByHash(ctx, HashRefreshToken(presentedRaw))
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			return nil
		}
		return err
	}
	return s.repo.RevokeFamily(ctx, current.FamilyID)
}
