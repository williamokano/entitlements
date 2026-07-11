// Package ports is the authentication module's public surface. Other modules
// (and the composition root) depend only on this package — never on the
// module's internal domain, service, or adapters.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// Published event types. Consumers subscribe to these on the bus; the strings
// are the stable contract.
const (
	EventUserRegistered  = "authn.user.registered"
	EventLoginSucceeded  = "authn.login.succeeded"
	EventLoginFailed     = "authn.login.failed"
	EventEmailVerified   = "authn.email.verified"
	EventPasswordChanged = "authn.password.changed"
	EventPasswordReset   = "authn.password.reset"
)

// UserRegistered is published when a new user registers.
type UserRegistered struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
}

// LoginSucceeded is published on a successful login.
type LoginSucceeded struct {
	UserID uuid.UUID `json:"user_id"`
}

// LoginFailed is published on a failed login. It carries the attempted email
// (normalized) and a coarse reason; it never reveals whether the email exists.
type LoginFailed struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// EmailVerified is published when a user verifies their email.
type EmailVerified struct {
	UserID uuid.UUID `json:"user_id"`
}

// PasswordChanged is published when a user changes their password while
// authenticated.
type PasswordChanged struct {
	UserID uuid.UUID `json:"user_id"`
}

// PasswordReset is published when a user completes a password recovery.
type PasswordReset struct {
	UserID uuid.UUID `json:"user_id"`
}

// Email is a message handed to an EmailSender. Bodies are pre-rendered by the
// module; the sender only transports them.
type Email struct {
	To       string
	Subject  string
	TextBody string
}

// EmailSender delivers transactional emails (verification links, reset links).
// The skeleton ships a dev adapter that logs the message; production deployments
// inject a real provider.
type EmailSender interface {
	Send(ctx context.Context, msg Email) error
}

// RateLimiter throttles authentication attempts. Allow records an attempt for a
// key (e.g. an email or client IP) and returns a non-nil error to block it. The
// default implementation is in-memory; production deployments swap in a shared
// store.
type RateLimiter interface {
	Allow(ctx context.Context, key string) error
}

// Identity is the verified subject of an access token, including the session
// (refresh-token family) it was issued on.
type Identity struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Email     string
}

// TokenVerifier validates an access token offline and returns its identity. The
// auth middleware (T-014) and other modules depend on this port rather than the
// concrete JWT verifier.
type TokenVerifier interface {
	Verify(rawAccessToken string) (Identity, error)
}
