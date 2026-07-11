// Package authentication owns identity only: users, credential factors
// (password first; TOTP/WebAuthn later), JWT access tokens with rotating
// refresh tokens, email verification, password recovery, sessions, and API
// keys for machine auth.
//
// It holds no permissions (see authorization) and no membership (see
// tenant). Hexagonal layout (domain / app / ports / adapters). Implemented
// across T-012, T-013, and T-014.
package authentication
