package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/authctx"
)

// ApplyPlanChange prorates a mid-period plan change and produces the billing
// artifact for it. It reads the old and new pinned plan-version prices, asks the
// configured ProrationStrategy for the signed adjustment over the remaining
// period, and then either issues a prorated invoice now (immediate_prorated) or
// stores a pending proration the next invoice will drain (credit_next_invoice).
// A zero adjustment (equal price, last day of the period, or the `none`
// strategy) produces nothing. It is the handler behind the idempotent
// SubscriptionPlanChanged consumer, so it runs once per plan change.
//
// All arithmetic is exact integer minor units — the strategy never uses floats.
func (s *Service) ApplyPlanChange(ctx context.Context, tenantID, subscriptionID, oldPlanVersionID, newPlanVersionID uuid.UUID, oldCycle, newCycle string) error {
	ctx = authctx.WithTenantID(ctx, tenantID)

	oldPV, err := s.catalog.GetPlanVersion(ctx, oldPlanVersionID)
	if err != nil {
		return err
	}
	newPV, err := s.catalog.GetPlanVersion(ctx, newPlanVersionID)
	if err != nil {
		return err
	}
	sub, err := s.subs.GetLiveForTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	oldUnit, _ := priceFor(oldPV.Prices, oldCycle)
	newUnit, _ := priceFor(newPV.Prices, newCycle)
	currency := newPV.Currency

	outcome := s.proration.Compute(ProrationInput{
		OldUnitMinor: oldUnit,
		NewUnitMinor: newUnit,
		Currency:     currency,
		PeriodStart:  sub.CurrentPeriodStart,
		PeriodEnd:    sub.CurrentPeriodEnd,
		Now:          s.clk.Now().UTC(),
	})
	if outcome.AmountMinor == 0 {
		return nil // nothing to bill (equal price, period boundary, or `none`)
	}

	description := fmt.Sprintf("Proration: plan change to %s v%d", newPV.PlanKey, newPV.Version)

	if outcome.Deferred {
		now := s.clk.Now().UTC()
		return s.uow.Do(ctx, func(ctx context.Context) error {
			return s.repo.CreatePendingProration(ctx, &domain.PendingProration{
				ID:             s.ids.New(),
				TenantID:       tenantID,
				SubscriptionID: subscriptionID,
				Description:    description,
				Key:            newPV.PlanKey,
				Version:        newPV.Version,
				AmountMinor:    outcome.AmountMinor,
				Currency:       currency,
				CreatedAt:      now,
			})
		})
	}

	return s.issueProrationInvoice(ctx, tenantID, subscriptionID, description, newPV.PlanKey, newPV.Version, outcome.AmountMinor, currency)
}

// issueProrationInvoice opens a one-line invoice for an immediate proration
// adjustment (a positive charge for an upgrade, a negative credit line for a
// downgrade), allocating the next per-tenant number in the same transaction.
func (s *Service) issueProrationInvoice(ctx context.Context, tenantID, subscriptionID uuid.UUID, description, key string, version int, amountMinor int64, currency string) error {
	now := s.clk.Now().UTC()
	line := domain.LineItem{
		ID:             s.ids.New(),
		Kind:           domain.LineKindProration,
		Description:    description,
		Key:            key,
		Version:        version,
		UnitPriceMinor: amountMinor,
		Quantity:       1,
		Currency:       currency,
		Position:       0,
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		number, err := s.repo.NextNumber(ctx, tenantID, "invoice")
		if err != nil {
			return err
		}
		inv, err := domain.NewInvoice(s.ids.New(), tenantID, subscriptionID, number, currency, []domain.LineItem{line}, 0, now)
		if err != nil {
			return err
		}
		if err := inv.Apply(domain.EventOpen, now); err != nil {
			return err
		}
		return s.repo.CreateInvoice(ctx, inv)
	})
}
