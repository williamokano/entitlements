package service

import (
	"context"

	"github.com/google/uuid"

	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// ChangePlan re-pins the current tenant's subscription to another published
// plan version. A price increase (upgrade) applies immediately; a decrease
// (downgrade) is stored as a scheduled change applied at the period boundary.
// Proration is not computed here — the published event carries old/new refs
// for billing.
func (s *Service) ChangePlan(ctx context.Context, newPlanVersionID uuid.UUID, cycle string) (View, error) {
	sub, err := s.liveForTenant(ctx)
	if err != nil {
		return View{}, err
	}
	newPV, err := s.catalog.GetPlanVersion(ctx, newPlanVersionID)
	if err != nil {
		return View{}, err
	}
	if newPV.Status != "published" {
		return View{}, apperr.Validation("plan version is not published")
	}
	if !cycleOffered(newPV, cycle) {
		return View{}, apperr.Validation("plan version has no price for this billing cycle")
	}
	oldPV, err := s.catalog.GetPlanVersion(ctx, sub.PlanVersionID)
	if err != nil {
		return View{}, err
	}

	now := s.clk.Now().UTC()
	if isUpgrade(oldPV, string(sub.BillingCycle), newPV, cycle) {
		if err := sub.ChangePlanNow(newPlanVersionID, domain.BillingCycle(cycle), now); err != nil {
			return View{}, err
		}
		if err := s.uow.Do(ctx, func(ctx context.Context) error {
			if err := s.repo.Update(ctx, sub); err != nil {
				return err
			}
			return s.publishPlanChange(ctx, sub)
		}); err != nil {
			return View{}, err
		}
		return toView(sub), nil
	}

	// Downgrade: schedule, apply at the boundary. No event until applied.
	if err := sub.ScheduleChange(newPlanVersionID, domain.BillingCycle(cycle), now); err != nil {
		return View{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		return s.repo.Update(ctx, sub)
	}); err != nil {
		return View{}, err
	}
	return toView(sub), nil
}

// CancelScheduledChange discards the current tenant's pending plan change.
func (s *Service) CancelScheduledChange(ctx context.Context) (View, error) {
	sub, err := s.liveForTenant(ctx)
	if err != nil {
		return View{}, err
	}
	if err := sub.CancelScheduledChange(s.clk.Now().UTC()); err != nil {
		return View{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		return s.repo.Update(ctx, sub)
	}); err != nil {
		return View{}, err
	}
	return toView(sub), nil
}

// ApplyScheduledChange re-pins a subscription to its scheduled plan version.
// This is the period-boundary hook the renewal job (T-021) calls; applying when
// nothing is scheduled is a no-op.
func (s *Service) ApplyScheduledChange(ctx context.Context, subscriptionID uuid.UUID) error {
	sub, err := s.repo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if !sub.ApplyScheduledChange(s.clk.Now().UTC()) {
		return nil
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, sub); err != nil {
			return err
		}
		return s.publishPlanChange(ctx, sub)
	})
}

// AttachAddon attaches a published addon version to the current tenant's
// subscription (or updates its quantity), enforcing plan compatibility and
// quantity rules.
func (s *Service) AttachAddon(ctx context.Context, addonVersionID uuid.UUID, quantity int) (View, error) {
	sub, err := s.liveForTenant(ctx)
	if err != nil {
		return View{}, err
	}
	av, err := s.catalog.GetAddonVersion(ctx, addonVersionID)
	if err != nil {
		return View{}, err
	}
	if av.Status != "published" {
		return View{}, apperr.Validation("addon version is not published")
	}
	pv, err := s.catalog.GetPlanVersion(ctx, sub.PlanVersionID)
	if err != nil {
		return View{}, err
	}
	if !av.CompatibleWith(pv.PlanKey) {
		return View{}, apperr.Validation("addon is not compatible with the subscription's plan")
	}
	if err := domain.ValidateAddonQuantity(av.QuantityAllowed, quantity); err != nil {
		return View{}, err
	}

	now := s.clk.Now().UTC()
	existing, err := s.repo.GetAddon(ctx, sub.ID, addonVersionID)
	if err != nil && apperr.KindOf(err) != apperr.KindNotFound {
		return View{}, err
	}
	action := ports.AddonAttached
	addon := &domain.Addon{ID: s.ids.New(), SubscriptionID: sub.ID, AddonVersionID: addonVersionID, Quantity: quantity, CreatedAt: now, UpdatedAt: now}
	if existing != nil {
		if existing.Quantity == quantity {
			return View{}, apperr.Conflict("addon already attached with this quantity")
		}
		action = ports.AddonQuantityChanged
		addon.ID, addon.CreatedAt = existing.ID, existing.CreatedAt
	}

	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.UpsertAddon(ctx, addon); err != nil {
			return err
		}
		return s.publishAddonChange(ctx, sub, addonVersionID, action, quantity)
	}); err != nil {
		return View{}, err
	}
	return s.GetForTenant(ctx)
}

// DetachAddon removes an attached addon version from the current tenant's
// subscription.
func (s *Service) DetachAddon(ctx context.Context, addonVersionID uuid.UUID) (View, error) {
	sub, err := s.liveForTenant(ctx)
	if err != nil {
		return View{}, err
	}
	if _, err := s.repo.GetAddon(ctx, sub.ID, addonVersionID); err != nil {
		return View{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.DeleteAddon(ctx, sub.ID, addonVersionID); err != nil {
			return err
		}
		return s.publishAddonChange(ctx, sub, addonVersionID, ports.AddonDetached, 0)
	}); err != nil {
		return View{}, err
	}
	return s.GetForTenant(ctx)
}

func (s *Service) publishPlanChange(ctx context.Context, sub *domain.Subscription) error {
	change := sub.PendingPlanChange()
	if change == nil {
		return nil
	}
	_, err := s.outbox.Publish(ctx, events.EventInput{
		TenantID: sub.TenantID,
		Module:   "subscription",
		Type:     ports.EventSubscriptionPlanChanged,
		Payload: ports.SubscriptionPlanChanged{
			SubscriptionID:   sub.ID,
			TenantID:         sub.TenantID,
			OldPlanVersionID: change.OldPlanVersionID,
			NewPlanVersionID: change.NewPlanVersionID,
			OldCycle:         string(change.OldCycle),
			NewCycle:         string(change.NewCycle),
			Timing:           change.Timing,
		},
	})
	return err
}

func (s *Service) publishAddonChange(ctx context.Context, sub *domain.Subscription, addonVersionID uuid.UUID, action string, quantity int) error {
	_, err := s.outbox.Publish(ctx, events.EventInput{
		TenantID: sub.TenantID,
		Module:   "subscription",
		Type:     ports.EventSubscriptionAddonChanged,
		Payload: ports.SubscriptionAddonChanged{
			SubscriptionID: sub.ID,
			TenantID:       sub.TenantID,
			AddonVersionID: addonVersionID,
			Action:         action,
			Quantity:       quantity,
		},
	})
	return err
}

// isUpgrade compares monthly-equivalent prices: equal or higher is an upgrade
// (applied immediately), lower is a downgrade (scheduled). Annual amounts are
// normalized to a monthly equivalent; custom cycles compare as-is.
func isUpgrade(oldPV catalogports.PlanVersionInfo, oldCycle string, newPV catalogports.PlanVersionInfo, newCycle string) bool {
	return monthlyEquivalent(newPV, newCycle) >= monthlyEquivalent(oldPV, oldCycle)
}

func monthlyEquivalent(pv catalogports.PlanVersionInfo, cycle string) int64 {
	for _, p := range pv.Prices {
		if p.Cycle != cycle {
			continue
		}
		if cycle == "annual" {
			return p.AmountMinor / 12
		}
		return p.AmountMinor
	}
	return 0
}
