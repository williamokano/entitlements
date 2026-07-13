// Package ports is the entitlements module's public surface: the reader other
// modules call to check a tenant's effective entitlements, and the summary-changed
// event it publishes whenever a tenant's effective set changes.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// EventEntitlementsSummaryChanged is published whenever a tenant's effective
// entitlement set changes (the analog of Stripe's
// entitlements.active_entitlement_summary.updated). It carries the full
// re-resolved set, so a phase-2 webhooks module can forward it verbatim.
const EventEntitlementsSummaryChanged = "entitlements.summary_changed"

// SummaryEntry is one feature's effective value and source in a summary event.
type SummaryEntry struct {
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// EntitlementsSummaryChanged is the payload of EventEntitlementsSummaryChanged.
type EntitlementsSummaryChanged struct {
	TenantID     uuid.UUID               `json:"tenant_id"`
	Entitlements map[string]SummaryEntry `json:"entitlements"`
}

// Entitlement is a single resolved entitlement other modules read.
type Entitlement struct {
	Key    string
	Value  any
	Source string
}

// EntitlementsReader is the entitlements module's facade for other modules: the
// resolved answer to "may this tenant do X, and how much".
type EntitlementsReader interface {
	// Get returns one feature's effective entitlement for a tenant.
	Get(ctx context.Context, tenantID uuid.UUID, key string) (Entitlement, error)
	// GetAll returns the tenant's whole effective set in one call.
	GetAll(ctx context.Context, tenantID uuid.UUID) (map[string]Entitlement, error)
}
