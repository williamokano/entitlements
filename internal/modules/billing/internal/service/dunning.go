package service

import (
	"context"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/domain"
	billingports "github.com/williamokano/entitlements/internal/modules/billing/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// ProcessDueDunning is the dunning job body: it scans for active dunning
// schedules whose next retry is due and retries the charge for each. It is
// registered on the jobs runner (advisory-lock elected, single executor per
// tick), so a retry runs once per tick. Because a retry uses a NEW attempt
// number for its provider idempotency key, a duplicate tick that races on the
// same schedule cannot double-charge: the schedule row's next_retry_at only
// moves forward once the retry commits.
//
// Retries are anchored at the initial failure instant (Dunning.FailedAt) plus
// the configured offsets, so under a frozen clock they fire at exactly the
// configured offsets and nowhere else. Returns the number of schedules retried.
func (s *Service) ProcessDueDunning(ctx context.Context) (int, error) {
	if s.provider == nil {
		return 0, apperr.Internal(errNoProvider)
	}
	now := s.clk.Now().UTC()
	due, err := s.repo.ListDueDunning(ctx, now)
	if err != nil {
		return 0, err
	}
	retried := 0
	for _, d := range due {
		if err := s.retryDunning(ctx, d); err != nil {
			return retried, err
		}
		retried++
	}
	return retried, nil
}

// retryDunning re-charges the open invoice for one due schedule. On success it
// pays the invoice (publishing billing.invoice_paid so the subscription advances
// its period) and emits billing.payment_recovered, then marks the schedule
// recovered. On a business decline it advances the schedule to the next offset,
// or exhausts it and emits billing.dunning_exhausted. A transport fault is
// returned as an error (the tick is retried) and leaves the schedule untouched.
func (s *Service) retryDunning(ctx context.Context, d *domain.Dunning) error {
	tctx := authctx.WithTenantID(ctx, d.TenantID)
	inv, err := s.repo.GetInvoice(tctx, d.TenantID, d.InvoiceID)
	if err != nil {
		return err
	}
	// If the invoice already left the open state (e.g. paid out of band), close the
	// schedule without charging.
	if inv.Status != domain.StatusOpen {
		now := s.clk.Now().UTC()
		return s.uow.Do(tctx, func(ctx context.Context) error {
			d.RecordRecovery(now)
			return s.repo.UpdateDunning(ctx, d)
		})
	}

	attempt := d.NextAttempt()
	res, err := s.provider.Charge(tctx, ChargeRequest{
		IdempotencyKey: ChargeIdempotencyKey(inv.ID, attempt),
		TenantID:       d.TenantID,
		InvoiceID:      inv.ID,
		AmountMinor:    inv.TotalMinor,
		Currency:       inv.Currency,
	})
	if err != nil {
		return err // transport fault: retry the tick, leave the schedule as-is
	}

	now := s.clk.Now().UTC()
	if res.Success {
		return s.uow.Do(tctx, func(ctx context.Context) error {
			if err := inv.Apply(domain.EventPay, now); err != nil {
				return err
			}
			if err := s.repo.UpdateInvoice(ctx, inv); err != nil {
				return err
			}
			if _, err := s.outbox.Publish(ctx, events.EventInput{
				TenantID: d.TenantID,
				Module:   "billing",
				Type:     billingports.EventInvoicePaid,
				Payload: billingports.InvoicePaid{
					InvoiceID:      inv.ID,
					TenantID:       d.TenantID,
					SubscriptionID: inv.SubscriptionID,
				},
			}); err != nil {
				return err
			}
			if _, err := s.outbox.Publish(ctx, events.EventInput{
				TenantID: d.TenantID,
				Module:   "billing",
				Type:     billingports.EventPaymentRecovered,
				Payload: billingports.PaymentRecovered{
					InvoiceID:      inv.ID,
					TenantID:       d.TenantID,
					SubscriptionID: inv.SubscriptionID,
				},
			}); err != nil {
				return err
			}
			d.RecordRecovery(now)
			return s.repo.UpdateDunning(ctx, d)
		})
	}

	// Business decline: advance the schedule (reschedule or exhaust).
	return s.uow.Do(tctx, func(ctx context.Context) error {
		d.RecordFailure(s.dunningOffsets, now)
		if err := s.repo.UpdateDunning(ctx, d); err != nil {
			return err
		}
		if d.Exhausted() {
			return s.publishDunningExhausted(ctx, d)
		}
		return nil
	})
}

// publishDunningExhausted emits billing.dunning_exhausted for a schedule whose
// retries are all used up, in the ambient transaction. The subscription module
// consumes it to move the subscription into its grace period.
func (s *Service) publishDunningExhausted(ctx context.Context, d *domain.Dunning) error {
	_, err := s.outbox.Publish(ctx, events.EventInput{
		TenantID: d.TenantID,
		Module:   "billing",
		Type:     billingports.EventDunningExhausted,
		Payload: billingports.DunningExhausted{
			InvoiceID:      d.InvoiceID,
			TenantID:       d.TenantID,
			SubscriptionID: d.SubscriptionID,
		},
	})
	return err
}
