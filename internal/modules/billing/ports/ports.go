// Package ports is the billing module's public surface: the events it publishes
// and the reader other modules call. Billing reports money outcomes as events;
// it never decides subscription policy.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// EventInvoicePaid is published when an invoice is paid. The subscription module
// consumes it (by this exact type) to advance a subscription's period. The
// charge flow that normally drives this lives in T-026; issuing/paying an
// invoice here already publishes it so the seam works.
const EventInvoicePaid = "billing.invoice_paid"

// InvoicePaid is the payload of EventInvoicePaid. SubscriptionID lets the
// subscription module route the event to the right subscription.
type InvoicePaid struct {
	InvoiceID      uuid.UUID `json:"invoice_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

// EventPaymentFailed is published when a renewal charge is declined. The invoice
// is left open and a dunning schedule is opened to retry it. The subscription
// module consumes it to move the subscription active → past_due.
const EventPaymentFailed = "billing.payment_failed"

// PaymentFailed is the payload of EventPaymentFailed.
type PaymentFailed struct {
	InvoiceID      uuid.UUID `json:"invoice_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
	Reason         string    `json:"reason"`
}

// EventPaymentRecovered is published when a dunning retry succeeds mid-schedule:
// the open invoice is paid and the retry loop stops. The subscription module
// consumes it to return a past_due/grace subscription to active.
const EventPaymentRecovered = "billing.payment_recovered"

// PaymentRecovered is the payload of EventPaymentRecovered.
type PaymentRecovered struct {
	InvoiceID      uuid.UUID `json:"invoice_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

// EventDunningExhausted is published when every scheduled dunning retry has
// failed: the invoice is left open and no further charges are attempted. The
// subscription module consumes it to move the subscription into its grace period
// (and then to suspended when that grace elapses).
const EventDunningExhausted = "billing.dunning_exhausted"

// DunningExhausted is the payload of EventDunningExhausted.
type DunningExhausted struct {
	InvoiceID      uuid.UUID `json:"invoice_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

// InvoiceInfo is the read model other modules see.
type InvoiceInfo struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	Number         int64
	Status         string
	Currency       string
	SubtotalMinor  int64
	TaxMinor       int64
	TotalMinor     int64
}

// BillingReader is the billing module's facade for other modules and the
// composition root. Reads are scoped to the tenant in context.
type BillingReader interface {
	GetInvoice(ctx context.Context, id uuid.UUID) (InvoiceInfo, error)
	ListInvoices(ctx context.Context) ([]InvoiceInfo, error)
}
