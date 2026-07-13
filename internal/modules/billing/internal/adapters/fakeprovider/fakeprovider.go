// Package fakeprovider is the billing module's default PaymentProvider adapter:
// an in-memory fake that auto-succeeds by default, can be programmed to fail
// specific operations, and records every call it receives so tests can assert on
// them. It is the shipped skeleton default (a real SaaS injects Stripe/Adyen/etc
// via billing.WithPaymentProvider); it is production-inert only because the
// default deployment keeps BILLING_DISABLED=true.
package fakeprovider

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
)

// Provider is a programmable, call-recording fake PaymentProvider. The zero value
// is not usable; construct one with New. It is safe for concurrent use.
type Provider struct {
	mu sync.Mutex

	// chargeFailure, when set, makes the next matching Charge decline (Success
	// false) with this reason instead of succeeding. failNextCharge is a one-shot
	// counter; failAllCharges makes every Charge decline.
	failNextCharge int
	failAllCharges bool
	failReason     string
	// chargeError, when set, makes Charge return a transport error (as if the
	// provider was unreachable) rather than a business decline.
	chargeError error

	charges  []ChargeCall
	refunds  []RefundCall
	attaches []AttachCall
	customer []CustomerCall
}

// ChargeCall records a single Charge invocation.
type ChargeCall struct {
	IdempotencyKey string
	TenantID       uuid.UUID
	InvoiceID      uuid.UUID
	AmountMinor    int64
	Currency       string
}

// RefundCall records a single Refund invocation.
type RefundCall struct {
	IdempotencyKey string
	ChargeRef      string
	AmountMinor    int64
	Currency       string
}

// AttachCall records a single AttachPaymentMethod invocation.
type AttachCall struct {
	CustomerRef string
	Token       string
}

// CustomerCall records a single CreateCustomer invocation.
type CustomerCall struct {
	TenantID uuid.UUID
	Email    string
}

// New builds a fake that auto-succeeds every operation.
func New() *Provider { return &Provider{} }

// Option programs the fake at construction time.
type Option func(*Provider)

// WithChargeFailure makes the next n Charge calls decline with reason. Pass a
// negative n to fail all charges.
func WithChargeFailure(n int, reason string) Option {
	return func(p *Provider) {
		p.failReason = reason
		if n < 0 {
			p.failAllCharges = true
			return
		}
		p.failNextCharge = n
	}
}

// FailNextCharges programs the next n Charge calls to decline with reason.
func (p *Provider) FailNextCharges(n int, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNextCharge = n
	p.failReason = reason
}

// FailAllCharges makes every subsequent Charge decline with reason.
func (p *Provider) FailAllCharges(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failAllCharges = true
	p.failReason = reason
}

// FailChargeWithError makes the next Charge return err (a transport fault, not a
// decline). Pass nil to clear it.
func (p *Provider) FailChargeWithError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chargeError = err
}

// CreateCustomer records the call and returns a deterministic customer ref.
func (p *Provider) CreateCustomer(_ context.Context, req service.CustomerRequest) (service.CustomerRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.customer = append(p.customer, CustomerCall{TenantID: req.TenantID, Email: req.Email})
	return service.CustomerRef{ID: "cus_fake_" + req.TenantID.String()}, nil
}

// AttachPaymentMethod records the call and echoes the token back as the stored
// reference. It never returns a raw PAN.
func (p *Provider) AttachPaymentMethod(_ context.Context, req service.AttachPaymentMethodRequest) (service.PaymentMethodRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.attaches = append(p.attaches, AttachCall{CustomerRef: req.CustomerRef, Token: req.Token})
	return service.PaymentMethodRef{Token: req.Token, Brand: req.Brand, Last4: req.Last4}, nil
}

// Charge records the call and returns success, a programmed decline, or a
// transport error, in that order of precedence.
func (p *Provider) Charge(_ context.Context, req service.ChargeRequest) (service.ChargeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.charges = append(p.charges, ChargeCall{
		IdempotencyKey: req.IdempotencyKey,
		TenantID:       req.TenantID,
		InvoiceID:      req.InvoiceID,
		AmountMinor:    req.AmountMinor,
		Currency:       req.Currency,
	})

	if p.chargeError != nil {
		err := p.chargeError
		return service.ChargeResult{}, err
	}
	if p.failAllCharges || p.failNextCharge > 0 {
		if p.failNextCharge > 0 {
			p.failNextCharge--
		}
		reason := p.failReason
		if reason == "" {
			reason = "card_declined"
		}
		return service.ChargeResult{Success: false, FailureReason: reason}, nil
	}
	return service.ChargeResult{Success: true, ProviderRef: "ch_fake_" + req.IdempotencyKey}, nil
}

// Refund records the call and returns success.
func (p *Provider) Refund(_ context.Context, req service.RefundRequest) (service.RefundResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refunds = append(p.refunds, RefundCall{
		IdempotencyKey: req.IdempotencyKey,
		ChargeRef:      req.ChargeRef,
		AmountMinor:    req.AmountMinor,
		Currency:       req.Currency,
	})
	return service.RefundResult{Success: true, ProviderRef: "re_fake_" + req.IdempotencyKey}, nil
}

// webhookBody is the fake provider's on-the-wire webhook shape. TranslateWebhook
// maps it to a provider-agnostic ProviderEvent.
type webhookBody struct {
	Event    string    `json:"event"`
	Charge   string    `json:"charge"`
	Invoice  uuid.UUID `json:"invoice"`
	Amount   int64     `json:"amount_minor"`
	Currency string    `json:"currency"`
}

// TranslateWebhook normalizes the fake's webhook JSON into a ProviderEvent.
func (p *Provider) TranslateWebhook(_ context.Context, in service.WebhookInput) (service.ProviderEvent, error) {
	var b webhookBody
	if err := json.Unmarshal(in.Body, &b); err != nil {
		return service.ProviderEvent{}, err
	}
	kind := service.ProviderEventUnknown
	switch b.Event {
	case "payment.succeeded":
		kind = service.ProviderEventChargeSucceeded
	case "payment.failed":
		kind = service.ProviderEventChargeFailed
	case "payment.refunded":
		kind = service.ProviderEventRefunded
	}
	return service.ProviderEvent{
		Kind:        kind,
		ProviderRef: b.Charge,
		InvoiceID:   b.Invoice,
		AmountMinor: b.Amount,
		Currency:    b.Currency,
	}, nil
}

// Charges returns a copy of the recorded Charge calls.
func (p *Provider) Charges() []ChargeCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ChargeCall(nil), p.charges...)
}

// ChargeCount returns how many Charge calls were recorded.
func (p *Provider) ChargeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.charges)
}

// Refunds returns a copy of the recorded Refund calls.
func (p *Provider) Refunds() []RefundCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]RefundCall(nil), p.refunds...)
}

// Attaches returns a copy of the recorded AttachPaymentMethod calls.
func (p *Provider) Attaches() []AttachCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]AttachCall(nil), p.attaches...)
}

// Customers returns a copy of the recorded CreateCustomer calls.
func (p *Provider) Customers() []CustomerCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]CustomerCall(nil), p.customer...)
}

// NewWith builds a fake configured by opts.
func NewWith(opts ...Option) *Provider {
	p := New()
	for _, o := range opts {
		o(p)
	}
	return p
}

// Ensure Provider satisfies the port at compile time.
var _ service.PaymentProvider = (*Provider)(nil)
