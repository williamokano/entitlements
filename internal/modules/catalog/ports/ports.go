// Package ports is the catalog module's public surface: the reader other modules
// call (subscriptions pin a plan version through it) and the events it publishes.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// Event type names published by the catalog module.
const (
	EventPlanVersionPublished = "catalog.plan_version.published"
	EventPlanArchived         = "catalog.plan.archived"
)

// PlanVersionPublished is published when a plan version is frozen.
type PlanVersionPublished struct {
	PlanID    uuid.UUID `json:"plan_id"`
	VersionID uuid.UUID `json:"version_id"`
	PlanKey   string    `json:"plan_key"`
	Version   int       `json:"version"`
}

// PlanArchived is published when a plan is archived.
type PlanArchived struct {
	PlanID  uuid.UUID `json:"plan_id"`
	PlanKey string    `json:"plan_key"`
}

// PriceInfo is a price in a plan-version snapshot (integer minor units).
type PriceInfo struct {
	Cycle       string `json:"cycle"`
	AmountMinor int64  `json:"amount_minor"`
}

// PlanVersionInfo is the frozen snapshot other modules read (e.g. a subscription
// pins this at creation and reads it forever after, unaffected by later plan
// changes).
type PlanVersionInfo struct {
	ID            uuid.UUID
	PlanID        uuid.UUID
	PlanKey       string
	Version       int
	Status        string
	Currency      string
	Prices        []PriceInfo
	TrialEnabled  bool
	TrialDays     int
	CardRequired  bool
	GraceDays     int
	FeatureGrants map[string]any
}

// CatalogReader is the catalog module's facade for other modules.
type CatalogReader interface {
	GetPlanVersion(ctx context.Context, id uuid.UUID) (PlanVersionInfo, error)
}
