package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// Override is a per-tenant manual entitlement override (a support gesture or a
// negotiated contract term). It wins over plan grants and addon deltas during
// resolution. Every override carries a mandatory reason and actor so the change
// is accountable (who / why), and an optional expires_at after which resolution
// ignores it and the expiry job reverts the tenant's effective set.
type Override struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	FeatureKey string
	Value      any
	Reason     string
	Actor      string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// LiveOverride is the value + expiry pair resolution reads for a feature.
// ExpiresAt is nil for an override that never expires.
type LiveOverride struct {
	Value     any
	ExpiresAt *time.Time
}

// NewOverride builds and validates a new override. A missing feature key,
// reason, or actor is a validation error: an override must always record what it
// targets and who set it and why.
func NewOverride(id, tenantID uuid.UUID, featureKey string, value any, reason, actor string, expiresAt *time.Time, now time.Time) (*Override, error) {
	o := &Override{
		ID:         id,
		TenantID:   tenantID,
		FeatureKey: featureKey,
		Value:      value,
		Reason:     reason,
		Actor:      actor,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	return o, nil
}

// Update applies mutable changes (value, expiry) and re-validates. Reason and
// actor are re-supplied so an edit is as accountable as a create.
func (o *Override) Update(value any, reason, actor string, expiresAt *time.Time) error {
	o.Value = value
	o.Reason = reason
	o.Actor = actor
	o.ExpiresAt = expiresAt
	return o.validate()
}

// validate enforces the override invariants: a target feature, a reason, and an
// actor are all mandatory.
func (o *Override) validate() error {
	if o.FeatureKey == "" {
		return apperr.Validation("override feature_key is required")
	}
	if o.Reason == "" {
		return apperr.Validation("override reason is required")
	}
	if o.Actor == "" {
		return apperr.Validation("override actor is required")
	}
	return nil
}
