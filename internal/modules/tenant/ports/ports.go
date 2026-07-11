// Package ports is the tenant module's public surface: the reader facade other
// modules call, the events it publishes, and the provisioning-hook contract.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// Event type names published by the tenant module.
const (
	EventTenantCreated     = "tenant.created"
	EventTenantSuspended   = "tenant.suspended"
	EventTenantReactivated = "tenant.reactivated"
	EventTenantDeleted     = "tenant.deleted"
)

// TenantCreated is the payload of EventTenantCreated.
type TenantCreated struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
}

// TenantStatusChanged is the payload of the suspend/reactivate/delete events.
type TenantStatusChanged struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Status   Status    `json:"status"`
}

// Status mirrors the tenant lifecycle state for consumers of the port.
type Status string

// Tenant lifecycle states.
const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusDeleted   Status = "deleted"
)

// TenantInfo is the read model other modules see.
type TenantInfo struct {
	ID     uuid.UUID
	Slug   string
	Name   string
	Status Status
}

// TenantReader is the tenant module's facade. Reads exclude soft-deleted
// tenants (they return a not-found error).
type TenantReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (TenantInfo, error)
	GetBySlug(ctx context.Context, slug string) (TenantInfo, error)
}

// ProvisioningHook performs one onboarding step after a tenant is created.
// Hooks run through the transactional outbox, so they execute after the create
// commits and are retried until they succeed. Each SaaS registers its own hooks
// (e.g. seed roles, create a trial subscription) at the composition root.
type ProvisioningHook interface {
	Name() string
	Provision(ctx context.Context, tenantID uuid.UUID) error
}
