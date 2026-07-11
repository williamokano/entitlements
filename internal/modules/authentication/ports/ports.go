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
	EventUserRegistered = "authn.user.registered"
	EventLoginSucceeded = "authn.login.succeeded"
	EventLoginFailed    = "authn.login.failed"
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

// RateLimiter throttles authentication attempts. Allow records an attempt for a
// key (e.g. an email or client IP) and returns a non-nil error to block it. The
// default implementation is in-memory; production deployments swap in a shared
// store.
type RateLimiter interface {
	Allow(ctx context.Context, key string) error
}

// Identity is the verified subject of an access token.
type Identity struct {
	UserID uuid.UUID
	Email  string
}

// TokenVerifier validates an access token offline and returns its identity. The
// auth middleware (T-014) and other modules depend on this port rather than the
// concrete JWT verifier.
type TokenVerifier interface {
	Verify(rawAccessToken string) (Identity, error)
}
