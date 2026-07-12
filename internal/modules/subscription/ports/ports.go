// Package ports is the subscription module's public surface: the reader other
// modules (entitlements, billing) call, and the transition event it publishes.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// EventSubscriptionTransitioned is published on every subscription state change
// (including creation). Consumers filter on the To state (e.g. entitlements
// re-resolves on active/canceled).
const EventSubscriptionTransitioned = "subscription.transitioned"

// SubscriptionTransitioned is the payload of EventSubscriptionTransitioned.
type SubscriptionTransitioned struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	PlanVersionID  uuid.UUID `json:"plan_version_id"`
	From           string    `json:"from"`
	To             string    `json:"to"`
	Event          string    `json:"event"`
}

// SubscriptionInfo is the read model other modules see.
type SubscriptionInfo struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	PlanVersionID      uuid.UUID
	BillingCycle       string
	Status             string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
}

// SubscriptionReader is the subscription module's facade for other modules.
type SubscriptionReader interface {
	GetLiveForTenant(ctx context.Context, tenantID uuid.UUID) (SubscriptionInfo, error)
}
