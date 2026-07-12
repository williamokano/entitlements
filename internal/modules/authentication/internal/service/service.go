// Package service holds the authentication use cases: register, login, refresh,
// and logout. Each mutation and its published event share one transaction (via
// the outbox), so state and event commit together.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/authentication/internal/security"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/audit"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// invalidCredentials is the single, constant-shape error returned for every
// login failure — unknown email or wrong password alike — so callers cannot
// enumerate which emails are registered.
var invalidCredentials = apperr.Unauthorized("invalid email or password")

// TokenPair is the result of a successful login or refresh.
type TokenPair struct {
	AccessToken     string
	RefreshToken    string
	AccessExpiresAt time.Time
}

// UserView is a read model of a registered user.
type UserView struct {
	ID     uuid.UUID
	Email  string
	Status string
}

// Service implements the authentication use cases.
type Service struct {
	uow             *postgres.UnitOfWork
	outbox          *events.Outbox
	users           domain.UserRepository
	refresh         *domain.RefreshService
	refreshRaw      domain.RefreshRepository
	authTokens      domain.AuthTokenRepository
	apiKeys         domain.APIKeyRepository
	signer          *security.Signer
	ids             id.Generator
	clk             clock.Clock
	refreshTTL      time.Duration
	limiter         ports.RateLimiter
	sender          ports.EmailSender
	audit           *audit.Writer
	baseURL         string
	verificationTTL time.Duration
	resetTTL        time.Duration
}

// Config carries the tunables and collaborators the recovery/session use cases
// need beyond the core login dependencies.
type Config struct {
	Sender          ports.EmailSender
	Audit           *audit.Writer
	BaseURL         string
	RefreshTTL      time.Duration
	VerificationTTL time.Duration
	ResetTTL        time.Duration
	Limiter         ports.RateLimiter
}

// New builds a Service.
func New(
	uow *postgres.UnitOfWork,
	outbox *events.Outbox,
	users domain.UserRepository,
	refreshRepo domain.RefreshRepository,
	authTokens domain.AuthTokenRepository,
	apiKeys domain.APIKeyRepository,
	signer *security.Signer,
	ids id.Generator,
	clk clock.Clock,
	cfg Config,
) *Service {
	return &Service{
		uow:             uow,
		outbox:          outbox,
		users:           users,
		refresh:         domain.NewRefreshService(refreshRepo),
		refreshRaw:      refreshRepo,
		authTokens:      authTokens,
		apiKeys:         apiKeys,
		signer:          signer,
		ids:             ids,
		clk:             clk,
		refreshTTL:      cfg.RefreshTTL,
		limiter:         cfg.Limiter,
		sender:          cfg.Sender,
		audit:           cfg.Audit,
		baseURL:         cfg.BaseURL,
		verificationTTL: cfg.VerificationTTL,
		resetTTL:        cfg.ResetTTL,
	}
}

// Register creates a user with a password credential and publishes
// authn.user.registered.
func (s *Service) Register(ctx context.Context, email, password string) (UserView, error) {
	now := s.clk.Now().UTC()
	user, err := domain.NewUser(s.ids.New(), email, now)
	if err != nil {
		return UserView{}, err
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return UserView{}, err
	}
	cred := &domain.Credential{
		ID:        s.ids.New(),
		UserID:    user.ID,
		Type:      domain.FactorPassword,
		Secret:    hash,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.users.CreateUser(ctx, user); err != nil {
			return err
		}
		if err := s.users.CreateCredential(ctx, cred); err != nil {
			return err
		}
		_, err := s.outbox.Publish(ctx, events.EventInput{
			Module:  "authentication",
			Type:    ports.EventUserRegistered,
			Payload: ports.UserRegistered{UserID: user.ID, Email: user.Email},
		})
		return err
	})
	if err != nil {
		return UserView{}, err
	}
	return UserView{ID: user.ID, Email: user.Email, Status: string(user.Status)}, nil
}

// Login verifies a password credential and, on success, issues an access token
// and a new refresh-token family. Failures return a constant-shape error and
// publish authn.login.failed; the rate-limit hook sees every attempt.
func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		// Treat a malformed email as a failed attempt, not a validation leak.
		normalized = ""
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(ctx, normalized); err != nil {
			return TokenPair{}, err
		}
	}

	user, verifyErr := s.authenticate(ctx, normalized, password)
	if verifyErr != nil {
		s.publishLoginFailed(ctx, normalized, "invalid_credentials")
		return TokenPair{}, invalidCredentials
	}

	pair, err := s.issueTokens(ctx, user, func(ctx context.Context) error {
		_, err := s.outbox.Publish(ctx, events.EventInput{
			Module:  "authentication",
			Type:    ports.EventLoginSucceeded,
			Payload: ports.LoginSucceeded{UserID: user.ID},
		})
		return err
	})
	if err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

// authenticate loads the user and verifies the password. Every failure path
// returns the same opaque error so the caller cannot distinguish "no such user"
// from "wrong password". To blunt timing-based user enumeration it verifies
// against a decoy hash even when the user does not exist.
func (s *Service) authenticate(ctx context.Context, email, password string) (*domain.User, error) {
	user, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		_, _ = security.VerifyPassword(password, decoyHash)
		return nil, invalidCredentials
	}
	if !user.Active() {
		return nil, invalidCredentials
	}
	cred, err := s.users.GetCredential(ctx, user.ID, domain.FactorPassword)
	if err != nil {
		_, _ = security.VerifyPassword(password, decoyHash)
		return nil, invalidCredentials
	}
	ok, err := security.VerifyPassword(password, cred.Secret)
	if err != nil || !ok {
		return nil, invalidCredentials
	}
	return user, nil
}

// Refresh rotates a refresh token and mints a fresh access token. Reuse of an
// already-rotated token revokes the whole family (see domain.RefreshService).
func (s *Service) Refresh(ctx context.Context, rawRefresh string) (TokenPair, error) {
	var (
		pair   TokenPair
		reused bool
	)
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		now := s.clk.Now().UTC()
		newRaw, err := security.GenerateOpaqueToken()
		if err != nil {
			return err
		}
		result, err := s.refresh.Rotate(ctx, rawRefresh, s.ids.New(), newRaw, now, now.Add(s.refreshTTL))
		if err != nil {
			return err
		}
		if result.Reused {
			// The family was revoked inside this transaction; commit that side
			// effect (return nil) and surface the rejection to the caller after.
			reused = true
			return nil
		}
		user, err := s.users.GetUserByID(ctx, result.Successor.UserID)
		if err != nil {
			return err
		}
		// The successor stays in the same family, so the session id is stable
		// across refreshes.
		access, err := s.signer.Sign(user.ID, result.Successor.FamilyID, user.Email, now)
		if err != nil {
			return err
		}
		pair = TokenPair{AccessToken: access, RefreshToken: newRaw, AccessExpiresAt: now.Add(s.accessTTL())}
		return nil
	})
	if err != nil {
		return TokenPair{}, err
	}
	if reused {
		return TokenPair{}, apperr.Unauthorized("refresh token is no longer valid")
	}
	return pair, nil
}

// Logout revokes the presented refresh token's family. It is idempotent.
func (s *Service) Logout(ctx context.Context, rawRefresh string) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		return s.refresh.Revoke(ctx, rawRefresh)
	})
}

// issueTokens mints an access token and persists a new refresh-token family,
// running extra (e.g. event) side effects in the same transaction. The access
// token carries the new family's id as its session id (sid).
func (s *Service) issueTokens(ctx context.Context, user *domain.User, extra func(context.Context) error) (TokenPair, error) {
	var pair TokenPair
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		now := s.clk.Now().UTC()
		rawRefresh, err := security.GenerateOpaqueToken()
		if err != nil {
			return err
		}
		token, err := s.refresh.Issue(ctx, s.ids.New(), user.ID, rawRefresh, now, now.Add(s.refreshTTL))
		if err != nil {
			return err
		}
		access, err := s.signer.Sign(user.ID, token.FamilyID, user.Email, now)
		if err != nil {
			return err
		}
		if extra != nil {
			if err := extra(ctx); err != nil {
				return err
			}
		}
		pair = TokenPair{AccessToken: access, RefreshToken: rawRefresh, AccessExpiresAt: now.Add(s.accessTTL())}
		return nil
	})
	if err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

func (s *Service) publishLoginFailed(ctx context.Context, email, reason string) {
	// Best-effort: a failed login must still return its error even if the event
	// cannot be recorded, so this runs in its own transaction and ignores errors.
	_ = s.uow.Do(ctx, func(ctx context.Context) error {
		_, err := s.outbox.Publish(ctx, events.EventInput{
			Module:  "authentication",
			Type:    ports.EventLoginFailed,
			Payload: ports.LoginFailed{Email: email, Reason: reason},
		})
		return err
	})
}

func (s *Service) accessTTL() time.Duration { return s.signer.TTL() }

// decoyHash is a fixed argon2id hash used to equalize the work done on the
// no-such-user path, mitigating timing-based enumeration. It is the hash of a
// random string no one knows.
var decoyHash = mustHash("decoy-password-not-a-real-secret")

func mustHash(s string) string {
	h, err := security.HashPassword(s)
	if err != nil {
		panic(errors.New("authentication: failed to build decoy hash"))
	}
	return h
}
