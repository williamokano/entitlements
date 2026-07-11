// Package authctx carries the authenticated principal and the resolved tenant
// through the request context.
//
// T-007 introduces the tenant-ID helpers (needed for correlated logging).
// T-011 extends this package with the Principal, the MustTenant guard, the
// WithSystemContext escape hatch, and the tenant-resolution middleware.
package authctx

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const tenantIDKey ctxKey = iota

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
