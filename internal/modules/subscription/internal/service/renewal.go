package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// ProcessDueRenewals scans live subscriptions whose billing period has ended and
// emits SubscriptionRenewalDue exactly once per period. When billing is disabled
// it also advances the period immediately (standing in for an instant payment);
// otherwise the period advances only when an InvoicePaid event arrives. Returns
// the number of subscriptions whose renewal was emitted this pass.
func (s *Service) ProcessDueRenewals(ctx context.Context) (int, error) {
	now := s.clk.Now().UTC()
	subs, err := s.repo.ListRenewable(ctx, now)
	if err != nil {
		return 0, err
	}
	emitted := 0
	for _, sub := range subs {
		if !sub.RenewalDue(now) {
			continue
		}
		if err := s.uow.Do(ctx, func(ctx context.Context) error {
			if err := s.publishRenewalDue(ctx, sub); err != nil {
				return err
			}
			sub.MarkRenewalEmitted(now)
			if s.billingDisabled && sub.AdvancePeriod(now) {
				if err := s.publishPlanChange(ctx, sub); err != nil {
					return err
				}
			}
			return s.repo.Update(ctx, sub)
		}); err != nil {
			return emitted, err
		}
		emitted++
	}
	return emitted, nil
}

// AdvancePeriodAfterPayment advances a subscription into its next period once its
// invoice is paid (the InvoicePaid consumer path when billing is enabled). It is
// idempotent: if the period has not ended, or the subscription is gone, it does
// nothing.
func (s *Service) AdvancePeriodAfterPayment(ctx context.Context, subscriptionID uuid.UUID) error {
	sub, err := s.repo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	now := s.clk.Now().UTC()
	if !sub.AdvancePeriod(now) {
		return nil
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, sub); err != nil {
			return err
		}
		return s.publishPlanChange(ctx, sub)
	})
}

// ProcessTrials scans trialing subscriptions, emitting TrialEnding for those
// nearing their trial end and resolving those that have reached it (converting
// to active or expiring per the plan's trial config). Returns counts of ending
// and resolved subscriptions.
func (s *Service) ProcessTrials(ctx context.Context) (ending int, resolved int, err error) {
	now := s.clk.Now().UTC()
	trials, err := s.repo.ListTrialing(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, sub := range trials {
		switch {
		case sub.TrialEnded(now):
			if err := s.resolveTrial(ctx, sub, now); err != nil {
				return ending, resolved, err
			}
			resolved++
		case sub.TrialEndingDue(now, s.trialEndingDays):
			if err := s.emitTrialEnding(ctx, sub, now); err != nil {
				return ending, resolved, err
			}
			ending++
		}
	}
	return ending, resolved, nil
}

// emitTrialEnding publishes TrialEnding and marks it emitted, atomically.
func (s *Service) emitTrialEnding(ctx context.Context, sub *domain.Subscription, now time.Time) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if _, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: sub.TenantID,
			Module:   "subscription",
			Type:     ports.EventSubscriptionTrialEnding,
			Payload: ports.SubscriptionTrialEnding{
				SubscriptionID: sub.ID,
				TenantID:       sub.TenantID,
				TrialEndsAt:    *sub.TrialEndsAt,
			},
		}); err != nil {
			return err
		}
		sub.MarkTrialEndingEmitted(now)
		return s.repo.Update(ctx, sub)
	})
}

// resolveTrial converts a trial to active (when the plan's trial needs no card)
// or expires it (when it required a card — no payment method is on file to
// charge). Both the state transition and the outcome event commit together.
func (s *Service) resolveTrial(ctx context.Context, sub *domain.Subscription, now time.Time) error {
	pv, err := s.catalog.GetPlanVersion(ctx, sub.PlanVersionID)
	if err != nil {
		return err
	}
	event, outcome := domain.EventActivate, ports.TrialConverted
	if pv.CardRequired {
		event, outcome = domain.EventExpire, ports.TrialExpired
	}
	if err := sub.Apply(event, "trial ended", "system", s.ids.New(), now); err != nil {
		return err
	}
	if outcome == ports.TrialConverted {
		sub.BeginPeriodAfterTrial(now)
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, sub); err != nil {
			return err
		}
		if err := s.recordAndPublish(ctx, sub); err != nil {
			return err
		}
		_, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: sub.TenantID,
			Module:   "subscription",
			Type:     ports.EventSubscriptionTrialEnded,
			Payload: ports.SubscriptionTrialEnded{
				SubscriptionID: sub.ID,
				TenantID:       sub.TenantID,
				Outcome:        outcome,
			},
		})
		return err
	})
}

// publishRenewalDue emits the renewal-due event in the ambient transaction.
func (s *Service) publishRenewalDue(ctx context.Context, sub *domain.Subscription) error {
	_, err := s.outbox.Publish(ctx, events.EventInput{
		TenantID: sub.TenantID,
		Module:   "subscription",
		Type:     ports.EventSubscriptionRenewalDue,
		Payload: ports.SubscriptionRenewalDue{
			SubscriptionID: sub.ID,
			TenantID:       sub.TenantID,
			PlanVersionID:  sub.PlanVersionID,
			BillingCycle:   string(sub.BillingCycle),
			PeriodEnd:      sub.CurrentPeriodEnd,
		},
	})
	return err
}
