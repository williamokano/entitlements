// Package domain holds the authentication aggregates and their rules. It is
// pure: only the standard library, uuid, and the platform error taxonomy
// (enforced by depguard). Password hashing and JWT signing live outside the
// domain (internal/security) because they pull in crypto dependencies the
// domain is not allowed to import.
package domain

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// Status is a user's lifecycle state.
type Status string

// User lifecycle states.
const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// FactorType identifies a credential kind. Password is the first; totp and
// webauthn slot in later against the same credentials table.
type FactorType string

// Credential factor kinds.
const (
	FactorPassword FactorType = "password"
	FactorTOTP     FactorType = "totp"
	FactorWebAuthn FactorType = "webauthn"
)

// User is a global identity. Membership in tenants lives in the tenant module;
// permissions live in authorization. A user carries no tenant scope.
type User struct {
	ID              uuid.UUID
	Email           string
	Status          Status
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewUser builds an active user with a normalized email.
func NewUser(id uuid.UUID, email string, now time.Time) (*User, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:        id,
		Email:     normalized,
		Status:    StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Active reports whether the user may authenticate.
func (u *User) Active() bool { return u.Status == StatusActive }

// EmailVerified reports whether the user's email has been verified.
func (u *User) EmailVerified() bool { return u.EmailVerifiedAt != nil }

// MarkEmailVerified records the email as verified at now (idempotent: an
// already-verified email keeps its original timestamp).
func (u *User) MarkEmailVerified(now time.Time) {
	if u.EmailVerifiedAt == nil {
		t := now
		u.EmailVerifiedAt = &t
	}
	u.UpdatedAt = now
}

// Credential is a stored authentication factor. Secret is the factor's verifier
// (an argon2id hash for password) — never a plaintext secret.
type Credential struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Type      FactorType
	Secret    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const emailMaxLen = 254

// NormalizeEmail lowercases, trims, and validates an email address.
func NormalizeEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", apperr.Validation("email must not be empty")
	}
	if len(s) > emailMaxLen {
		return "", apperr.Validation("email is too long")
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", apperr.Validation("email is not a valid address")
	}
	return s, nil
}

// UserRepository persists users and their credentials. Reads exclude disabled
// users only where noted; lookups here return any non-deleted user.
type UserRepository interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	MarkEmailVerified(ctx context.Context, userID uuid.UUID, verifiedAt time.Time) error
	CreateCredential(ctx context.Context, c *Credential) error
	GetCredential(ctx context.Context, userID uuid.UUID, typ FactorType) (*Credential, error)
	UpdateCredentialSecret(ctx context.Context, userID uuid.UUID, typ FactorType, secret string, updatedAt time.Time) error
}
