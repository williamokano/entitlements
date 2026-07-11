// Package authentication owns identity only: users, credential factors
// (password first; TOTP/WebAuthn later), JWT access tokens with rotating
// refresh tokens, and (in later tasks) email verification, recovery, sessions,
// and API keys.
//
// It holds no permissions (see authorization) and no membership (see tenant).
// Hexagonal layout: domain / service / ports / adapters, with domain and
// service unexported under internal/. Cross-module access is via the ports
// package only.
package authentication

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/authentication/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/authentication/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/authentication/internal/security"
	"github.com/williamokano/entitlements/internal/modules/authentication/internal/service"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/config"
)

// moduleConfig is the authentication module's own configuration, loaded from the
// environment independently of the central config (the documented per-module
// config pattern).
type moduleConfig struct {
	JWTIssuer  string        `env:"AUTH_JWT_ISSUER" envDefault:"entitlements"`
	JWTKeyID   string        `env:"AUTH_JWT_KID" envDefault:"dev"`
	JWTSeed    string        `env:"AUTH_JWT_ED25519_SEED"` // base64 std, 32 bytes; empty => ephemeral dev key
	AccessTTL  time.Duration `env:"AUTH_ACCESS_TTL" envDefault:"15m"`
	RefreshTTL time.Duration `env:"AUTH_REFRESH_TTL" envDefault:"720h"`

	// Rate limiting for login attempts.
	RateLimitMax    int           `env:"AUTH_LOGIN_RATE_MAX" envDefault:"10"`
	RateLimitWindow time.Duration `env:"AUTH_LOGIN_RATE_WINDOW" envDefault:"1m"`
}

// Module wires the authentication module from platform dependencies.
type Module struct {
	deps     app.Deps
	svc      *service.Service
	handler  http.Handler
	verifier *security.Verifier
	limiter  ports.RateLimiter
}

// Option customizes the module.
type Option func(*Module)

// WithRateLimiter overrides the default in-memory login rate limiter (e.g. with
// a shared-store implementation, or a recording hook in tests).
func WithRateLimiter(l ports.RateLimiter) Option {
	return func(m *Module) { m.limiter = l }
}

// New constructs the authentication module. It fails only if the environment
// carries a malformed signing key.
func New(deps app.Deps, opts ...Option) (*Module, error) {
	cfg, err := config.Parse[moduleConfig]()
	if err != nil {
		return nil, err
	}

	priv, err := loadOrGenerateKey(cfg.JWTSeed)
	if err != nil {
		return nil, err
	}
	signer := security.NewSigner(cfg.JWTKeyID, priv, cfg.JWTIssuer, cfg.AccessTTL)
	verifier := security.NewVerifier(
		map[string]ed25519.PublicKey{cfg.JWTKeyID: signer.PublicKey()},
		cfg.JWTIssuer,
	)

	m := &Module{
		deps:     deps,
		verifier: verifier,
		limiter:  NewMemoryRateLimiter(deps.Clock, cfg.RateLimitMax, cfg.RateLimitWindow),
	}
	for _, opt := range opts {
		opt(m)
	}

	repo := pgadapter.New(deps.Pool)
	m.svc = service.New(deps.UnitOfWork, deps.Outbox, repo, repo, signer, deps.IDs, deps.Clock, cfg.RefreshTTL, m.limiter)
	m.handler = rest.New(m.svc)
	return m, nil
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "auth" }

// Handler is the module's HTTP handler, mounted under /api/v1/auth.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions is empty: the authentication module publishes events but does
// not consume any (yet).
func (m *Module) Subscriptions() []app.Subscription { return nil }

// Verifier returns the module's public token verifier so other modules and the
// auth middleware (T-014) can validate access tokens offline.
func (m *Module) Verifier() ports.TokenVerifier { return tokenVerifier{v: m.verifier} }

// tokenVerifier adapts the concrete security.Verifier to the ports.TokenVerifier
// interface, keeping the internal type out of the module's public surface.
type tokenVerifier struct{ v *security.Verifier }

func (t tokenVerifier) Verify(raw string) (ports.Identity, error) {
	claims, err := t.v.Verify(raw)
	if err != nil {
		return ports.Identity{}, err
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("%w: bad subject", security.ErrInvalidToken)
	}
	return ports.Identity{UserID: uid, Email: claims.Email}, nil
}

// loadOrGenerateKey builds the Ed25519 signing key from a base64 seed, or
// generates an ephemeral key when no seed is configured (development only —
// tokens do not survive a restart).
func loadOrGenerateKey(seedB64 string) (ed25519.PrivateKey, error) {
	if seedB64 == "" {
		_, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, fmt.Errorf("authentication: generate ephemeral key: %w", err)
		}
		return priv, nil
	}
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return nil, fmt.Errorf("authentication: decode AUTH_JWT_ED25519_SEED: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("authentication: AUTH_JWT_ED25519_SEED must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
