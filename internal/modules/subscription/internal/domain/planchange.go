package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// Plan-change timings: an upgrade applies immediately; a downgrade waits for
// the period boundary.
const (
	TimingImmediate = "immediate"
	TimingPeriodEnd = "period_end"
)

// ScheduledChange is a plan change deferred to the period boundary.
type ScheduledChange struct {
	PlanVersionID uuid.UUID
	BillingCycle  BillingCycle
}

// PlanChange records one applied plan change (old → new refs), for the service
// to publish. Produced by ChangePlanNow / ApplyScheduledChange like pending
// transitions are.
type PlanChange struct {
	OldPlanVersionID uuid.UUID
	NewPlanVersionID uuid.UUID
	OldCycle         BillingCycle
	NewCycle         BillingCycle
	Timing           string
}

// canChangePlan guards all plan-change entry points.
func (s *Subscription) canChangePlan(to uuid.UUID, cycle BillingCycle) error {
	if s.Terminal() {
		return apperr.Conflict("subscription is terminal")
	}
	if !validCycle(cycle) {
		return apperr.Validation("invalid billing cycle")
	}
	if s.PlanVersionID == to && s.BillingCycle == cycle {
		return apperr.Validation("subscription is already on this plan version and cycle")
	}
	return nil
}

// ChangePlanNow re-pins the subscription to a new plan version immediately (an
// upgrade). Any scheduled change is superseded and cleared. The applied change
// is recorded as pending for the service to publish.
func (s *Subscription) ChangePlanNow(to uuid.UUID, cycle BillingCycle, now time.Time) error {
	if err := s.canChangePlan(to, cycle); err != nil {
		return err
	}
	s.pendingChange = &PlanChange{
		OldPlanVersionID: s.PlanVersionID, NewPlanVersionID: to,
		OldCycle: s.BillingCycle, NewCycle: cycle,
		Timing: TimingImmediate,
	}
	s.PlanVersionID = to
	s.BillingCycle = cycle
	s.Scheduled = nil
	s.UpdatedAt = now
	return nil
}

// ScheduleChange stores a plan change to be applied at the period boundary (a
// downgrade). Nothing else changes now — the current pin stays in force.
func (s *Subscription) ScheduleChange(to uuid.UUID, cycle BillingCycle, now time.Time) error {
	if err := s.canChangePlan(to, cycle); err != nil {
		return err
	}
	s.Scheduled = &ScheduledChange{PlanVersionID: to, BillingCycle: cycle}
	s.UpdatedAt = now
	return nil
}

// CancelScheduledChange discards the pending scheduled change.
func (s *Subscription) CancelScheduledChange(now time.Time) error {
	if s.Scheduled == nil {
		return apperr.Conflict("subscription has no scheduled plan change")
	}
	s.Scheduled = nil
	s.UpdatedAt = now
	return nil
}

// ApplyScheduledChange re-pins to the scheduled plan version (called at the
// period boundary by the renewal job). Reports whether a change was applied.
func (s *Subscription) ApplyScheduledChange(now time.Time) bool {
	if s.Scheduled == nil {
		return false
	}
	s.pendingChange = &PlanChange{
		OldPlanVersionID: s.PlanVersionID, NewPlanVersionID: s.Scheduled.PlanVersionID,
		OldCycle: s.BillingCycle, NewCycle: s.Scheduled.BillingCycle,
		Timing: TimingPeriodEnd,
	}
	s.PlanVersionID = s.Scheduled.PlanVersionID
	s.BillingCycle = s.Scheduled.BillingCycle
	s.Scheduled = nil
	s.UpdatedAt = now
	return true
}

// PendingPlanChange returns and clears the plan change produced by the last
// ChangePlanNow/ApplyScheduledChange, for the service to publish.
func (s *Subscription) PendingPlanChange() *PlanChange {
	c := s.pendingChange
	s.pendingChange = nil
	return c
}

// Addon is an addon version attached to a subscription with a quantity.
type Addon struct {
	ID             uuid.UUID
	SubscriptionID uuid.UUID
	AddonVersionID uuid.UUID
	Quantity       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ValidateAddonQuantity enforces the quantity rules: at least 1, and exactly 1
// unless the addon version allows quantities.
func ValidateAddonQuantity(quantityAllowed bool, quantity int) error {
	if quantity < 1 {
		return apperr.Validation("addon quantity must be at least 1")
	}
	if !quantityAllowed && quantity != 1 {
		return apperr.Validation("addon does not allow quantities")
	}
	return nil
}

// AddonRepository persists a subscription's attached addons.
type AddonRepository interface {
	GetAddon(ctx context.Context, subscriptionID, addonVersionID uuid.UUID) (*Addon, error)
	UpsertAddon(ctx context.Context, a *Addon) error
	DeleteAddon(ctx context.Context, subscriptionID, addonVersionID uuid.UUID) error
	ListAddons(ctx context.Context, subscriptionID uuid.UUID) ([]*Addon, error)
}
