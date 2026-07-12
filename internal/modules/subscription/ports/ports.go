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

// EventSubscriptionPlanChanged is published when a subscription is re-pinned to
// a different plan version — immediately for upgrades, at the period boundary
// for scheduled downgrades. It carries old/new refs so billing can prorate and
// entitlements can re-resolve; no proration is computed here.
const EventSubscriptionPlanChanged = "subscription.plan_changed"

// SubscriptionPlanChanged is the payload of EventSubscriptionPlanChanged.
type SubscriptionPlanChanged struct {
	SubscriptionID   uuid.UUID `json:"subscription_id"`
	TenantID         uuid.UUID `json:"tenant_id"`
	OldPlanVersionID uuid.UUID `json:"old_plan_version_id"`
	NewPlanVersionID uuid.UUID `json:"new_plan_version_id"`
	OldCycle         string    `json:"old_cycle"`
	NewCycle         string    `json:"new_cycle"`
	Timing           string    `json:"timing"` // immediate | period_end
}

// EventSubscriptionAddonChanged is published when an addon is attached,
// detached, or its quantity changes.
const EventSubscriptionAddonChanged = "subscription.addon_changed"

// Addon-change actions.
const (
	AddonAttached        = "attached"
	AddonDetached        = "detached"
	AddonQuantityChanged = "quantity_changed"
)

// SubscriptionAddonChanged is the payload of EventSubscriptionAddonChanged.
type SubscriptionAddonChanged struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	AddonVersionID uuid.UUID `json:"addon_version_id"`
	Action         string    `json:"action"` // attached | detached | quantity_changed
	Quantity       int       `json:"quantity"`
}

// EventSubscriptionRenewalDue is published when a subscription's period ends and
// it is time to bill the next period. Billing consumes it to issue an invoice;
// exactly one is emitted per period.
const EventSubscriptionRenewalDue = "subscription.renewal_due"

// SubscriptionRenewalDue is the payload of EventSubscriptionRenewalDue.
type SubscriptionRenewalDue struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	PlanVersionID  uuid.UUID `json:"plan_version_id"`
	BillingCycle   string    `json:"billing_cycle"`
	PeriodEnd      time.Time `json:"period_end"`
}

// EventSubscriptionTrialEnding is published a configured number of days before a
// trial ends.
const EventSubscriptionTrialEnding = "subscription.trial_ending"

// SubscriptionTrialEnding is the payload of EventSubscriptionTrialEnding.
type SubscriptionTrialEnding struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	TrialEndsAt    time.Time `json:"trial_ends_at"`
}

// EventSubscriptionTrialEnded is published when a trial reaches its end and is
// resolved (converted to active or expired per the plan's trial config).
const EventSubscriptionTrialEnded = "subscription.trial_ended"

// Trial-ended outcomes.
const (
	TrialConverted = "converted"
	TrialExpired   = "expired"
)

// SubscriptionTrialEnded is the payload of EventSubscriptionTrialEnded.
type SubscriptionTrialEnded struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	Outcome        string    `json:"outcome"` // converted | expired
}

// EventBillingInvoicePaid is the billing event the subscription module consumes
// to advance a subscription's period when billing is enabled. Billing owns this
// event; the name and minimal shape are declared here until the billing module
// (T-025/026) lands and publishes it.
const EventBillingInvoicePaid = "billing.invoice_paid"

// BillingInvoicePaid is the (partial) payload the subscription module reads from
// an invoice-paid event.
type BillingInvoicePaid struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
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
