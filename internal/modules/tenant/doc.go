// Package tenant is the tenancy module: the root of isolation that every
// other module is scoped by.
//
// It owns tenant lifecycle (create/update/suspend/soft-delete), extensible
// JSONB settings, membership and invitations, and a hookable provisioning
// pipeline driven by the TenantCreated event. Internally it follows the
// hexagonal layout (domain / app / ports / adapters). Implemented across
// T-010, T-011, and T-015.
package tenant
