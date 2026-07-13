package service

import (
	"time"
)

// ProrationInput describes a mid-period plan change to prorate. All money is
// integer minor units; the period and change instant are UTC times.
type ProrationInput struct {
	OldUnitMinor int64
	NewUnitMinor int64
	Currency     string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	Now          time.Time
}

// ProrationOutcome is the result of applying a strategy. AmountMinor is the
// signed adjustment to bill for the remainder of the period (positive = an extra
// charge for an upgrade, negative = a credit for a downgrade, zero = nothing).
// Deferred reports whether the amount should be applied to the next invoice
// rather than billed immediately.
type ProrationOutcome struct {
	AmountMinor int64
	Deferred    bool
}

// ProrationStrategy computes how a mid-period plan change is billed. The service
// depends on the interface; the concrete strategy is selected from config at
// construction. It is a pure function of its input — no I/O, no clock, exact
// integer arithmetic. It is modeled like the TaxCalculator/PaymentProvider
// driven ports: the service depends on the interface, the composition root
// selects the implementation.
type ProrationStrategy interface {
	// Name is the strategy's config identifier.
	Name() string
	// Compute returns the billing outcome for the change described by in.
	Compute(in ProrationInput) ProrationOutcome
}

// Proration strategy identifiers (config values for BILLING_PRORATION_STRATEGY).
const (
	ProrationImmediate         = "immediate_prorated"
	ProrationNone              = "none"
	ProrationCreditNextInvoice = "credit_next_invoice"
)

// NewProrationStrategy returns the strategy named by name, defaulting to
// immediate_prorated for the empty or an unknown name.
func NewProrationStrategy(name string) ProrationStrategy {
	switch name {
	case ProrationNone:
		return NoProration{}
	case ProrationCreditNextInvoice:
		return CreditNextInvoiceProration{}
	default:
		return ImmediateProration{}
	}
}

// proratedDifference is the exact prorated adjustment for a plan change, in
// minor units: the price difference scaled by the fraction of the period that
// remains, using integer arithmetic only (truncation toward zero, no floats).
//
//	amount = (newUnit - oldUnit) * remainingDays / totalDays
//
// remainingDays is clamped to [0, totalDays]: on the first day of the period the
// full difference is billed (remaining == total), on the last day nothing is
// (remaining == 0), and an equal price yields zero. The division truncates
// toward zero, so the customer is never over-billed by a rounding fraction.
func proratedDifference(in ProrationInput) int64 {
	totalDays := wholeDays(in.PeriodStart, in.PeriodEnd)
	if totalDays <= 0 {
		return 0
	}
	remainingDays := wholeDays(in.Now, in.PeriodEnd)
	if remainingDays < 0 {
		remainingDays = 0
	}
	if remainingDays > totalDays {
		remainingDays = totalDays
	}
	diff := in.NewUnitMinor - in.OldUnitMinor
	return diff * remainingDays / totalDays
}

// wholeDays is the number of whole 24h days from a to b (negative if b precedes
// a), truncated. Proration is day-granular by design.
func wholeDays(a, b time.Time) int64 {
	return int64(b.Sub(a) / (24 * time.Hour))
}

// ImmediateProration bills the prorated difference on an invoice now.
type ImmediateProration struct{}

// Name identifies the strategy.
func (ImmediateProration) Name() string { return ProrationImmediate }

// Compute returns the prorated difference, billed immediately.
func (ImmediateProration) Compute(in ProrationInput) ProrationOutcome {
	return ProrationOutcome{AmountMinor: proratedDifference(in), Deferred: false}
}

// NoProration bills nothing for a plan change: the new price simply applies from
// the next period.
type NoProration struct{}

// Name identifies the strategy.
func (NoProration) Name() string { return ProrationNone }

// Compute always returns a zero, non-deferred outcome.
func (NoProration) Compute(ProrationInput) ProrationOutcome {
	return ProrationOutcome{}
}

// CreditNextInvoiceProration computes the same prorated difference as the
// immediate strategy but defers it to the next invoice instead of billing now.
type CreditNextInvoiceProration struct{}

// Name identifies the strategy.
func (CreditNextInvoiceProration) Name() string { return ProrationCreditNextInvoice }

// Compute returns the prorated difference, deferred to the next invoice.
func (CreditNextInvoiceProration) Compute(in ProrationInput) ProrationOutcome {
	return ProrationOutcome{AmountMinor: proratedDifference(in), Deferred: true}
}
