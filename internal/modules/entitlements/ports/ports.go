// Package ports is the entitlements module's public surface: the reader other
// modules call to check a tenant's effective entitlements, and the summary-changed
// event it publishes whenever a tenant's effective set changes.
package ports

import (
	"context"
	"time"

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

// EventEntitlementLimitWarning is published the first time a consume crosses a
// soft limit within a period (before ≤ limit < after). It fires exactly once per
// crossing — the crossing consume claims a per-period latch — so downstream
// (notifications, webhooks) can nudge the tenant without spamming.
const EventEntitlementLimitWarning = "entitlements.limit_warning"

// EventEntitlementExceeded is published when a tenant's effective limit shrinks
// below its current usage (a downgrade or override reduction). It is emitted once
// per shrink and never blocks reads; only future consumes are gated by the new
// limit.
const EventEntitlementExceeded = "entitlements.exceeded"

// UsageEvent is the shared payload of the limit-warning and exceeded events: the
// feature, the tenant, and the usage/limit at the moment the event fired.
type UsageEvent struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	FeatureKey string    `json:"feature_key"`
	Used       int64     `json:"used"`
	Limit      int64     `json:"limit"`
	Period     string    `json:"period"`
}

// Usage is a metered feature's current consumption for a tenant: how much of the
// effective limit has been used in the current period. Unlimited is set when the
// feature carries no numeric limit (e.g. an unknown feature under allow policy),
// in which case Limit is not meaningful.
type Usage struct {
	Key       string
	Used      int64
	Limit     int64
	Unlimited bool
	Period    string
	Behavior  string
}

// UsageReader is the entitlements module's metered-quota facade for other
// modules: consume against a limit (atomically, honoring hard/soft behavior),
// release, and read current usage. Consume returns a KindQuotaExceeded error
// when a hard limit would be breached.
type UsageReader interface {
	ConsumeQuota(ctx context.Context, tenantID uuid.UUID, key string, n int64) (Usage, error)
	ReleaseQuota(ctx context.Context, tenantID uuid.UUID, key string, n int64) (Usage, error)
	GetUsage(ctx context.Context, tenantID uuid.UUID, key string) (Usage, error)
	ListUsage(ctx context.Context, tenantID uuid.UUID) ([]Usage, error)
}

// Entitlement is a single resolved entitlement other modules read. ExpiresAt is
// set only when the effective value comes from a time-bound override, so callers
// can see when it will revert.
type Entitlement struct {
	Key       string
	Value     any
	Source    string
	ExpiresAt *time.Time
}

// EntitlementsReader is the entitlements module's facade for other modules: the
// resolved answer to "may this tenant do X, and how much".
type EntitlementsReader interface {
	// Get returns one feature's effective entitlement for a tenant.
	Get(ctx context.Context, tenantID uuid.UUID, key string) (Entitlement, error)
	// GetAll returns the tenant's whole effective set in one call.
	GetAll(ctx context.Context, tenantID uuid.UUID) (map[string]Entitlement, error)
}
