// Package domain holds the invoice aggregate, its lifecycle state machine, and
// the credit note. It is pure: only the standard library, uuid, and the platform
// error taxonomy. All money is integer minor units; there is no float arithmetic
// anywhere in this package.
package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// Status is an invoice's lifecycle state.
type Status string

// Invoice states. paid, void and uncollectible are terminal.
const (
	StatusDraft         Status = "draft"
	StatusOpen          Status = "open"
	StatusPaid          Status = "paid"
	StatusVoid          Status = "void"
	StatusUncollectible Status = "uncollectible"
)

// Event triggers an invoice transition.
type Event string

// Transition events.
const (
	EventOpen              Event = "open"               // draft -> open (issue)
	EventPay               Event = "pay"                // open -> paid
	EventVoid              Event = "void"               // draft/open -> void
	EventMarkUncollectible Event = "mark_uncollectible" // open -> uncollectible
)

// transitions is the single source of truth for the invoice state machine: for a
// given status, which event leads to which next status. Any (status, event) pair
// not present here is illegal.
var transitions = map[Status]map[Event]Status{
	StatusDraft: {
		EventOpen: StatusOpen,
		EventVoid: StatusVoid,
	},
	StatusOpen: {
		EventPay:               StatusPaid,
		EventVoid:              StatusVoid,
		EventMarkUncollectible: StatusUncollectible,
	},
	StatusPaid:          {}, // terminal
	StatusVoid:          {}, // terminal
	StatusUncollectible: {}, // terminal
}

// AllStatuses enumerates every status for exhaustive testing.
func AllStatuses() []Status {
	return []Status{StatusDraft, StatusOpen, StatusPaid, StatusVoid, StatusUncollectible}
}

// AllEvents lists every event.
func AllEvents() []Event {
	return []Event{EventOpen, EventPay, EventVoid, EventMarkUncollectible}
}

// NextStatus returns the status an event leads to from `from`, and whether the
// transition is legal.
func NextStatus(from Status, event Event) (Status, bool) {
	to, ok := transitions[from][event]
	return to, ok
}

// LineKind distinguishes a plan charge from an addon charge.
type LineKind string

// Line kinds.
const (
	LineKindPlan      LineKind = "plan"
	LineKindAddon     LineKind = "addon"
	LineKindProration LineKind = "proration"
)

// LineItem is a snapshotted charge on an invoice. Every field is a copy taken at
// issuance (the catalog key, version, unit price, quantity, currency) so the line
// is never re-derived from the catalog afterwards.
type LineItem struct {
	ID             uuid.UUID
	Kind           LineKind
	Description    string
	Key            string // plan_or_addon_key
	Version        int
	UnitPriceMinor int64
	Quantity       int
	Currency       string
	Position       int
}

// Amount is the line total in minor units: unit price × quantity. Both operands
// are integers, so the product is exact — no rounding, no floats.
func (li LineItem) Amount() int64 {
	return li.UnitPriceMinor * int64(li.Quantity)
}

// Subtotal sums line amounts in minor units. Pure integer addition.
func Subtotal(lines []LineItem) int64 {
	var sum int64
	for _, li := range lines {
		sum += li.Amount()
	}
	return sum
}

// Invoice is the billing document for a subscription's period. Its totals and
// lines are frozen once it is issued.
type Invoice struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	Number         int64
	Status         Status
	Currency       string
	SubtotalMinor  int64
	TaxMinor       int64
	TotalMinor     int64
	IssuedAt       *time.Time
	Lines          []LineItem
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewInvoice builds a draft invoice from snapshotted lines and a tax amount
// (already computed by the TaxCalculator, in minor units). It validates the
// currency, requires at least one line, and rejects any line whose currency
// differs from the invoice currency. Totals are computed in minor units:
// subtotal = Σ line amounts, total = subtotal + tax.
func NewInvoice(id, tenantID, subscriptionID uuid.UUID, number int64, currency string, lines []LineItem, taxMinor int64, now time.Time) (*Invoice, error) {
	if currency == "" {
		return nil, apperr.Validation("invoice currency is required")
	}
	if len(lines) == 0 {
		return nil, apperr.Validation("invoice has no line items")
	}
	if taxMinor < 0 {
		return nil, apperr.Validation("tax cannot be negative")
	}
	for i := range lines {
		if lines[i].Currency != currency {
			return nil, apperr.Validation(fmt.Sprintf("line %d currency %q does not match invoice currency %q", i, lines[i].Currency, currency))
		}
		if lines[i].Quantity < 1 {
			return nil, apperr.Validation("line quantity must be at least 1")
		}
	}
	subtotal := Subtotal(lines)
	return &Invoice{
		ID:             id,
		TenantID:       tenantID,
		SubscriptionID: subscriptionID,
		Number:         number,
		Status:         StatusDraft,
		Currency:       currency,
		SubtotalMinor:  subtotal,
		TaxMinor:       taxMinor,
		TotalMinor:     subtotal + taxMinor,
		Lines:          lines,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Apply moves the invoice through the state machine. An illegal (status, event)
// pair is rejected as a conflict and the status is left unchanged. Opening the
// invoice stamps IssuedAt.
func (inv *Invoice) Apply(event Event, now time.Time) error {
	to, ok := NextStatus(inv.Status, event)
	if !ok {
		return apperr.Conflict(fmt.Sprintf("cannot apply %q to a %s invoice", event, inv.Status))
	}
	inv.Status = to
	inv.UpdatedAt = now
	if event == EventOpen {
		issued := now
		inv.IssuedAt = &issued
	}
	return nil
}

// Terminal reports whether the invoice is in a terminal state.
func (inv *Invoice) Terminal() bool {
	return inv.Status == StatusPaid || inv.Status == StatusVoid || inv.Status == StatusUncollectible
}

// PendingProration is a deferred plan-change adjustment (the credit_next_invoice
// strategy) awaiting the subscription's next invoice, which drains it into a
// proration line. AmountMinor is signed (positive charge, negative credit).
type PendingProration struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	Description    string
	Key            string
	Version        int
	AmountMinor    int64
	Currency       string
	CreatedAt      time.Time
}

// Repository persists invoices, their snapshotted line items, credit notes, and
// the per-tenant number sequences.
type Repository interface {
	// NextNumber atomically allocates the next per-tenant number for kind
	// ("invoice" or "credit_note") within the ambient transaction. It is gapless
	// and dup-free under concurrency (a row lock serializes issuers per tenant).
	NextNumber(ctx context.Context, tenantID uuid.UUID, kind string) (int64, error)
	CreateInvoice(ctx context.Context, inv *Invoice) error
	UpdateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoice(ctx context.Context, tenantID, id uuid.UUID) (*Invoice, error)
	ListInvoices(ctx context.Context, tenantID uuid.UUID) ([]*Invoice, error)
	CreateCreditNote(ctx context.Context, cn *CreditNote) error
	ListCreditNotes(ctx context.Context, tenantID, invoiceID uuid.UUID) ([]*CreditNote, error)
	// CreatePaymentMethod persists a tokenized payment method. The schema's
	// token-only CHECK constraint is the last line of defense against a raw PAN
	// ever landing in storage.
	CreatePaymentMethod(ctx context.Context, pm *PaymentMethod) error
	ListPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]*PaymentMethod, error)
	// CreateDunning opens a dunning schedule for a declined renewal charge.
	CreateDunning(ctx context.Context, d *Dunning) error
	// UpdateDunning persists a schedule's status/attempt/next-retry after a retry.
	UpdateDunning(ctx context.Context, d *Dunning) error
	// ListDueDunning returns active schedules whose next retry is due at now,
	// across all tenants (the dunning job scans globally).
	ListDueDunning(ctx context.Context, now time.Time) ([]*Dunning, error)
	// CreatePendingProration stores a deferred plan-change adjustment.
	CreatePendingProration(ctx context.Context, p *PendingProration) error
	// ListPendingProrations returns a subscription's unapplied deferred prorations.
	ListPendingProrations(ctx context.Context, subscriptionID uuid.UUID) ([]*PendingProration, error)
	// MarkProrationsApplied records that the given prorations were drained into an
	// invoice, so they are never billed twice.
	MarkProrationsApplied(ctx context.Context, ids []uuid.UUID, invoiceID uuid.UUID) error
}
