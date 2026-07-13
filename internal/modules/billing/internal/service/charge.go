package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/domain"
	billingports "github.com/williamokano/entitlements/internal/modules/billing/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// errNoProvider is returned when a charge-flow method is called on a service
// wired without a PaymentProvider (a misconfiguration).
var errNoProvider = errors.New("billing: no payment provider configured")

// renewalChargeAttempt is the charge attempt number for the initial renewal
// charge. Dunning retries (T-027) use higher numbers, each yielding a distinct
// stable idempotency key.
const renewalChargeAttempt = 1

// ChargeForRenewal is the money side of a subscription renewal: it issues the
// invoice for the tenant's live subscription, charges the payment provider with
// an idempotency key derived from (invoice, attempt), and on success pays the
// invoice (publishing billing.invoice_paid so the subscription advances its
// period), or on decline publishes billing.payment_failed and leaves the invoice
// open. It is the handler behind the idempotent SubscriptionRenewalDue consumer,
// so a redelivered renewal event is dropped before this runs — and the provider
// idempotency key defends the charge itself.
//
// A transport fault from the provider is returned as an error so the delivery is
// retried; a business decline is not an error (the invoice stays open and the
// failure is reported as an event).
//
// The renewal targets the tenant's single live subscription (which Issue reads),
// so only the tenant is needed to drive the charge.
func (s *Service) ChargeForRenewal(ctx context.Context, tenantID uuid.UUID) error {
	if s.provider == nil {
		return apperr.Internal(errNoProvider)
	}
	ctx = authctx.WithTenantID(ctx, tenantID)

	view, err := s.Issue(ctx)
	if err != nil {
		return err
	}

	res, err := s.provider.Charge(ctx, ChargeRequest{
		IdempotencyKey: ChargeIdempotencyKey(view.ID, renewalChargeAttempt),
		TenantID:       tenantID,
		InvoiceID:      view.ID,
		AmountMinor:    view.TotalMinor,
		Currency:       view.Currency,
	})
	if err != nil {
		return err // transport fault: retry the whole delivery
	}
	if res.Success {
		_, err := s.Pay(ctx, view.ID)
		return err
	}
	return s.publishPaymentFailed(ctx, tenantID, view, res.FailureReason)
}

// publishPaymentFailed emits billing.payment_failed for a declined renewal
// charge and opens the dunning schedule that will retry the charge, both in one
// transaction. The invoice is intentionally left open. The dunning schedule is
// anchored at the failure instant, so its retries are due at the configured
// offsets from that point regardless of when the scan runs.
func (s *Service) publishPaymentFailed(ctx context.Context, tenantID uuid.UUID, view View, reason string) error {
	now := s.clk.Now().UTC()
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if _, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenantID,
			Module:   "billing",
			Type:     billingports.EventPaymentFailed,
			Payload: billingports.PaymentFailed{
				InvoiceID:      view.ID,
				TenantID:       tenantID,
				SubscriptionID: view.SubscriptionID,
				Reason:         reason,
			},
		}); err != nil {
			return err
		}
		d := domain.NewDunning(s.ids.New(), tenantID, view.ID, view.SubscriptionID, s.dunningOffsets, now)
		if err := s.repo.CreateDunning(ctx, d); err != nil {
			return err
		}
		// No offsets configured ⇒ the schedule opens already exhausted; report it.
		if d.Exhausted() {
			return s.publishDunningExhausted(ctx, d)
		}
		return nil
	})
}

// PaymentMethodView is a read model of a stored payment method: display metadata
// and the opaque token reference, never card data.
type PaymentMethodView struct {
	ID          uuid.UUID
	CustomerRef string
	Token       string
	Brand       string
	Last4       string
}

// AttachPaymentMethod tokenizes and stores a payment method for the current
// tenant: it ensures a provider customer exists, attaches the (already
// gateway-tokenized) method to get a token reference, and persists tokens only.
// A raw card number is rejected by the domain guard and the schema CHECK before
// anything is stored.
func (s *Service) AttachPaymentMethod(ctx context.Context, token, brand, last4 string) (PaymentMethodView, error) {
	if s.provider == nil {
		return PaymentMethodView{}, apperr.Internal(errNoProvider)
	}
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return PaymentMethodView{}, apperr.Validation("tenant not specified")
	}

	cust, err := s.provider.CreateCustomer(ctx, CustomerRequest{TenantID: tenantID})
	if err != nil {
		return PaymentMethodView{}, err
	}
	ref, err := s.provider.AttachPaymentMethod(ctx, AttachPaymentMethodRequest{
		CustomerRef: cust.ID,
		Token:       token,
		Brand:       brand,
		Last4:       last4,
	})
	if err != nil {
		return PaymentMethodView{}, err
	}

	now := s.clk.Now().UTC()
	pm, err := domain.NewPaymentMethod(s.ids.New(), tenantID, cust.ID, ref.Token, ref.Brand, ref.Last4, now)
	if err != nil {
		return PaymentMethodView{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		return s.repo.CreatePaymentMethod(ctx, pm)
	}); err != nil {
		return PaymentMethodView{}, err
	}
	return PaymentMethodView{
		ID: pm.ID, CustomerRef: pm.CustomerRef, Token: pm.Token, Brand: pm.Brand, Last4: pm.Last4,
	}, nil
}

// ListPaymentMethods returns the current tenant's stored payment methods.
func (s *Service) ListPaymentMethods(ctx context.Context) ([]PaymentMethodView, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return nil, apperr.Validation("tenant not specified")
	}
	pms, err := s.repo.ListPaymentMethods(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethodView, 0, len(pms))
	for _, pm := range pms {
		out = append(out, PaymentMethodView{
			ID: pm.ID, CustomerRef: pm.CustomerRef, Token: pm.Token, Brand: pm.Brand, Last4: pm.Last4,
		})
	}
	return out, nil
}
