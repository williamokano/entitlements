// Package authctx carries the authenticated principal and the resolved tenant
// through the request context.
//
// T-007 introduced the tenant-ID helpers; T-011 adds the Principal, the tenant
// claim (set by the auth layer, read by the tenant-resolution middleware), the
// MustTenant repository guard, and the WithSystemContext escape hatch for
// internal callers (jobs, the outbox relay, provisioning). T-012+ populate the
// Principal.
package authctx

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type ctxKey int

const (
	tenantIDKey ctxKey = iota
	tenantClaimKey
	principalKey
	systemKey
)

// ErrNoTenant is returned by MustTenant when no tenant is present in context.
var ErrNoTenant = errors.New("authctx: no tenant in context")

// PrincipalKind classifies the authenticated caller.
type PrincipalKind string

// Principal kinds.
const (
	PrincipalUser    PrincipalKind = "user"
	PrincipalMachine PrincipalKind = "machine"
	PrincipalSystem  PrincipalKind = "system"
)

// Principal is the authenticated caller. It is populated by the authentication
// middleware (T-012/T-014); Subject is the user or API-key ID.
type Principal struct {
	Kind    PrincipalKind
	Subject string
}

// WithTenantID stores the resolved tenant ID in the context.
func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// TenantID returns the tenant ID carried by the context, and whether one was
// present.
func TenantID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantIDKey).(uuid.UUID)
	return id, ok
}

// MustTenant returns the tenant ID, or ErrNoTenant if none is present. It never
// returns a zero tenant ID with a nil error, so tenant-scoped repositories can
// rely on it to refuse to run without a tenant.
func MustTenant(ctx context.Context) (uuid.UUID, error) {
	if id, ok := TenantID(ctx); ok && id != uuid.Nil {
		return id, nil
	}
	return uuid.Nil, ErrNoTenant
}

// WithTenantClaim stores a tenant identifier asserted by the authentication
// layer (e.g. a JWT claim). The tenant-resolution middleware prefers it over
// the request header or subdomain.
func WithTenantClaim(ctx context.Context, identifier string) context.Context {
	return context.WithValue(ctx, tenantClaimKey, identifier)
}

// TenantClaim returns the tenant identifier asserted by the auth layer.
func TenantClaim(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(tenantClaimKey).(string)
	return v, ok && v != ""
}

// WithPrincipal stores the authenticated principal.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext returns the authenticated principal, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// WithSystemContext marks the context as system-privileged, so the
// tenant-resolution middleware lets it pass without a tenant. It is set only by
// internal callers (never derived from a request), so an external caller can
// never obtain it.
func WithSystemContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, systemKey, true)
}

// IsSystem reports whether the context is system-privileged.
func IsSystem(ctx context.Context) bool {
	v, _ := ctx.Value(systemKey).(bool)
	return v
}
