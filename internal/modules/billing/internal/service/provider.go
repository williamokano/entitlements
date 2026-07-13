package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// PaymentProvider is the billing module's outbound (driven) port to a payment
// gateway. It is modeled like the TaxCalculator port: the service depends on the
// interface and a concrete adapter is injected at the composition root. The
// skeleton ships a fake adapter (fakeprovider) that auto-succeeds by default and
// can be programmed to fail; a real SaaS plugs in Stripe/Adyen/etc.
//
// The provider never sees raw card data through this module: cards are tokenized
// by the gateway (AttachPaymentMethod returns a token reference) and only the
// token is ever persisted.
type PaymentProvider interface {
	// CreateCustomer registers (or returns) the provider-side customer for a
	// tenant and returns its opaque reference.
	CreateCustomer(ctx context.Context, req CustomerRequest) (CustomerRef, error)
	// AttachPaymentMethod tokenizes and attaches a payment method to a customer,
	// returning a token reference. Implementations must never echo back a raw PAN.
	AttachPaymentMethod(ctx context.Context, req AttachPaymentMethodRequest) (PaymentMethodRef, error)
	// Charge captures an amount against a customer's payment method. The
	// IdempotencyKey makes retries with the same key a no-op at the provider, so
	// a redelivered renewal never double-charges.
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
	// Refund reverses (part of) a prior charge.
	Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
	// TranslateWebhook normalizes a raw provider webhook into a ProviderEvent the
	// billing module understands, insulating callers from provider-specific shapes.
	TranslateWebhook(ctx context.Context, in WebhookInput) (ProviderEvent, error)
}

// CustomerRequest asks the provider to register a tenant's customer.
type CustomerRequest struct {
	TenantID uuid.UUID
	Email    string
}

// CustomerRef is the provider's opaque handle for a customer.
type CustomerRef struct {
	ID string
}

// AttachPaymentMethodRequest attaches a payment method (already tokenized by the
// gateway's client SDK) to a customer. Token is a provider token reference, never
// a raw card number.
type AttachPaymentMethodRequest struct {
	CustomerRef string
	Token       string
	Brand       string
	Last4       string
}

// PaymentMethodRef is the provider's token reference for a stored payment method.
type PaymentMethodRef struct {
	Token string
	Brand string
	Last4 string
}

// ChargeRequest captures money against a customer. Money is integer minor units
// plus a currency code, never a float. IdempotencyKey is derived from
// (invoice, attempt) so it is stable across retries.
type ChargeRequest struct {
	IdempotencyKey string
	TenantID       uuid.UUID
	CustomerRef    string
	InvoiceID      uuid.UUID
	AmountMinor    int64
	Currency       string
}

// ChargeResult is the outcome of a Charge. Success is true when captured; on a
// decline Success is false and FailureReason explains it (no error is returned
// for a business decline — only for transport/provider faults).
type ChargeResult struct {
	Success       bool
	ProviderRef   string
	FailureReason string
}

// RefundRequest reverses a prior charge.
type RefundRequest struct {
	IdempotencyKey string
	ChargeRef      string
	AmountMinor    int64
	Currency       string
}

// RefundResult is the outcome of a Refund.
type RefundResult struct {
	Success     bool
	ProviderRef string
}

// WebhookInput is a raw provider webhook: the event type header (if any) and the
// raw JSON body as received.
type WebhookInput struct {
	Type string
	Body []byte
}

// ProviderEvent is a normalized payment event translated from a provider
// webhook. Kind is a provider-agnostic classification (see the Provider* kinds).
type ProviderEvent struct {
	Kind        string
	ProviderRef string
	InvoiceID   uuid.UUID
	AmountMinor int64
	Currency    string
}

// Normalized provider event kinds.
const (
	ProviderEventChargeSucceeded = "charge_succeeded"
	ProviderEventChargeFailed    = "charge_failed"
	ProviderEventRefunded        = "refunded"
	ProviderEventUnknown         = "unknown"
)

// ChargeIdempotencyKey derives a provider idempotency key from an invoice and a
// charge attempt number. It is a pure function: the same (invoice, attempt)
// always yields the same key, so a retried charge for the same attempt reuses the
// key and the provider treats it as a no-op. Dunning (T-027) uses higher attempt
// numbers to make each retry a distinct, idempotent charge.
func ChargeIdempotencyKey(invoiceID uuid.UUID, attempt int) string {
	return fmt.Sprintf("charge:%s:%d", invoiceID, attempt)
}
