package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// APIKey is a machine (server-to-server) credential bound to a tenant. Only the
// argon2id hash of the secret is stored; the public prefix identifies the key
// for lookup before the single hash verification.
type APIKey struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	Name       string
	Prefix     string
	SecretHash string
	Scopes     []string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Revoked reports whether the key has been revoked.
func (k *APIKey) Revoked() bool { return k.RevokedAt != nil }

// HasScope reports whether the key carries the given scope.
func (k *APIKey) HasScope(scope string) bool {
	for _, s := range k.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// APIKeyRepository persists API keys.
type APIKeyRepository interface {
	Insert(ctx context.Context, k *APIKey) error
	GetByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*APIKey, error)
	Revoke(ctx context.Context, id, tenantID uuid.UUID, at time.Time) error
	TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}
