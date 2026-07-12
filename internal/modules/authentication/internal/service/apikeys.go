package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/authentication/internal/security"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// invalidAPIKey is the constant error for any unusable API key (malformed,
// unknown, revoked, or wrong secret) so callers cannot probe key state.
var invalidAPIKey = apperr.Unauthorized("invalid api key")

// APIKeyView is a read model of an API key. It never includes the secret.
type APIKeyView struct {
	ID         uuid.UUID
	Name       string
	Prefix     string
	Scopes     []string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// CreateAPIKey mints a tenant-bound API key and returns the view plus the raw
// secret, which is shown exactly once — only its argon2id hash is stored.
func (s *Service) CreateAPIKey(ctx context.Context, tenantID uuid.UUID, name string, scopes []string) (APIKeyView, string, error) {
	if name == "" {
		return APIKeyView{}, "", apperr.Validation("api key name must not be empty")
	}
	material, err := security.GenerateAPIKey()
	if err != nil {
		return APIKeyView{}, "", err
	}
	hash, err := security.HashPassword(material.Secret)
	if err != nil {
		return APIKeyView{}, "", err
	}
	if scopes == nil {
		scopes = []string{}
	}
	key := &domain.APIKey{
		ID:         s.ids.New(),
		TenantID:   tenantID,
		Name:       name,
		Prefix:     material.Prefix,
		SecretHash: hash,
		Scopes:     scopes,
		CreatedAt:  s.clk.Now().UTC(),
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		return s.apiKeys.Insert(ctx, key)
	}); err != nil {
		return APIKeyView{}, "", err
	}
	return toAPIKeyView(key), material.Full, nil
}

// ListAPIKeys returns a tenant's active API keys (never their secrets).
func (s *Service) ListAPIKeys(ctx context.Context, tenantID uuid.UUID) ([]APIKeyView, error) {
	keys, err := s.apiKeys.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	views := make([]APIKeyView, 0, len(keys))
	for _, k := range keys {
		views = append(views, toAPIKeyView(k))
	}
	return views, nil
}

// RevokeAPIKey revokes a tenant's API key. Revocation takes effect immediately.
func (s *Service) RevokeAPIKey(ctx context.Context, tenantID, keyID uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		return s.apiKeys.Revoke(ctx, keyID, tenantID, s.clk.Now().UTC())
	})
}

// AuthenticateAPIKey validates a presented API key and returns the machine
// identity it authorizes. It looks the key up by its public prefix, then does a
// single argon2id verification of the secret. Every failure path returns the
// same opaque error.
func (s *Service) AuthenticateAPIKey(ctx context.Context, rawKey string) (ports.MachineIdentity, error) {
	prefix, secret, ok := security.ParseAPIKey(rawKey)
	if !ok {
		return ports.MachineIdentity{}, invalidAPIKey
	}
	key, err := s.apiKeys.GetByPrefix(ctx, prefix)
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			return ports.MachineIdentity{}, invalidAPIKey
		}
		return ports.MachineIdentity{}, err
	}
	if key.Revoked() {
		return ports.MachineIdentity{}, invalidAPIKey
	}
	valid, err := security.VerifyPassword(secret, key.SecretHash)
	if err != nil || !valid {
		return ports.MachineIdentity{}, invalidAPIKey
	}
	// Best-effort last-used telemetry; never fail the request over it.
	_ = s.apiKeys.TouchLastUsed(ctx, key.ID, s.clk.Now().UTC())

	return ports.MachineIdentity{KeyID: key.ID, TenantID: key.TenantID, Scopes: key.Scopes}, nil
}

func toAPIKeyView(k *domain.APIKey) APIKeyView {
	return APIKeyView{
		ID:         k.ID,
		Name:       k.Name,
		Prefix:     k.Prefix,
		Scopes:     k.Scopes,
		LastUsedAt: k.LastUsedAt,
		RevokedAt:  k.RevokedAt,
		CreatedAt:  k.CreatedAt,
	}
}
