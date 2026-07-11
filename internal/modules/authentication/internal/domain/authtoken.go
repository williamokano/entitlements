package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TokenPurpose scopes a single-use token so it can only be redeemed for its
// intended action (a verification token can never be used as a reset token).
type TokenPurpose string

// Token purposes.
const (
	PurposeEmailVerification TokenPurpose = "email_verification"
	PurposePasswordReset     TokenPurpose = "password_reset"
)

// AuthToken is a single-use, expiring token for email verification or password
// reset. Only its hash is stored; the raw value is delivered out of band (email)
// and never persisted.
type AuthToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Purpose    TokenPurpose
	Hash       string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// NewAuthToken builds an unconsumed token.
func NewAuthToken(id, userID uuid.UUID, purpose TokenPurpose, rawToken string, now, expiresAt time.Time) *AuthToken {
	return &AuthToken{
		ID:        id,
		UserID:    userID,
		Purpose:   purpose,
		Hash:      HashToken(rawToken),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
}

// Redeemable reports whether the token can still be used at now: unconsumed and
// unexpired.
func (t *AuthToken) Redeemable(now time.Time) bool {
	return t.ConsumedAt == nil && !now.After(t.ExpiresAt)
}

// AuthTokenRepository persists single-use tokens. Consume marks a token used in
// the ambient transaction, so redemption and the action it authorizes commit
// together (guaranteeing single use).
type AuthTokenRepository interface {
	Insert(ctx context.Context, t *AuthToken) error
	FindByHash(ctx context.Context, purpose TokenPurpose, hash string) (*AuthToken, error)
	Consume(ctx context.Context, id uuid.UUID, consumedAt time.Time) error
}
