package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
)

// MarkPaymentFailed moves a subscription active → past_due when billing reports a
// declined renewal charge. It is the billing.payment_failed consumer. If the
// subscription is not active (a payment_failed for a subscription already in
// past_due/grace or a terminal state), it is a no-op, so redeliveries and
// out-of-order dunning events never illegally transition the machine.
func (s *Service) MarkPaymentFailed(ctx context.Context, subscriptionID uuid.UUID) error {
	sub, err := s.repo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if _, ok := domain.NextState(sub.Status, domain.EventPaymentFailed); !ok {
		return nil
	}
	_, err = s.apply(ctx, sub, domain.EventPaymentFailed, "billing: payment failed")
	return err
}

// EnterGraceOnDunningExhausted moves a subscription past_due → grace when billing
// exhausts its charge retries, stamping the grace deadline from the pinned plan
// version's grace days. It is the billing.dunning_exhausted consumer; a no-op
// unless the subscription is past_due.
func (s *Service) EnterGraceOnDunningExhausted(ctx context.Context, subscriptionID uuid.UUID) error {
	sub, err := s.repo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if _, ok := domain.NextState(sub.Status, domain.EventEnterGrace); !ok {
		return nil
	}
	pv, err := s.catalog.GetPlanVersion(ctx, sub.PlanVersionID)
	if err != nil {
		return err
	}
	now := s.clk.Now().UTC()
	if err := sub.Apply(domain.EventEnterGrace, "billing: dunning exhausted", actorOf(ctx), s.ids.New(), now); err != nil {
		return err
	}
	sub.SetGraceWindow(pv.GraceDays, now)
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, sub); err != nil {
			return err
		}
		return s.recordAndPublish(ctx, sub)
	})
}

// RecoverPayment returns a past_due/grace subscription to active when billing
// reports a recovered payment (a dunning retry that succeeded). It is the
// billing.payment_recovered consumer; a no-op unless the subscription is in a
// dunning state.
func (s *Service) RecoverPayment(ctx context.Context, subscriptionID uuid.UUID) error {
	sub, err := s.repo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if _, ok := domain.NextState(sub.Status, domain.EventPaymentOK); !ok {
		return nil
	}
	now := s.clk.Now().UTC()
	if err := sub.Apply(domain.EventPaymentOK, "billing: payment recovered", actorOf(ctx), s.ids.New(), now); err != nil {
		return err
	}
	sub.ClearGraceWindow(now)
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, sub); err != nil {
			return err
		}
		return s.recordAndPublish(ctx, sub)
	})
}

// ProcessGraceExpiries is the grace scan job body: it suspends every subscription
// whose grace period has elapsed (grace → suspended). Returns the number of
// subscriptions suspended this pass.
func (s *Service) ProcessGraceExpiries(ctx context.Context) (int, error) {
	now := s.clk.Now().UTC()
	subs, err := s.repo.ListGraceExpired(ctx, now)
	if err != nil {
		return 0, err
	}
	suspended := 0
	for _, sub := range subs {
		if err := sub.Apply(domain.EventSuspend, "grace period elapsed", "system", s.ids.New(), now); err != nil {
			return suspended, err
		}
		sub.ClearGraceWindow(now)
		if err := s.uow.Do(ctx, func(ctx context.Context) error {
			if err := s.repo.Update(ctx, sub); err != nil {
				return err
			}
			return s.recordAndPublish(ctx, sub)
		}); err != nil {
			return suspended, err
		}
		suspended++
	}
	return suspended, nil
}
