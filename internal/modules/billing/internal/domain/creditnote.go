package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// CreditNote references an invoice and negates (part of) its amount. The stored
// AmountMinor is negative: a credit is a reduction of what the tenant owes.
type CreditNote struct {
	ID          uuid.UUID
	InvoiceID   uuid.UUID
	TenantID    uuid.UUID
	Number      int64
	AmountMinor int64 // negative — the credited magnitude, negated
	Currency    string
	Reason      string
	CreatedAt   time.Time
}

// NewCreditNote builds a credit note that credits creditMinor (a positive
// magnitude in minor units) against an invoice. The stored amount is the
// negation (-creditMinor), so the credit note negates invoice amounts. A reason
// is mandatory.
func NewCreditNote(id, invoiceID, tenantID uuid.UUID, number, creditMinor int64, currency, reason string, now time.Time) (*CreditNote, error) {
	if creditMinor <= 0 {
		return nil, apperr.Validation("credit amount must be positive")
	}
	if reason == "" {
		return nil, apperr.Validation("credit note requires a reason")
	}
	if currency == "" {
		return nil, apperr.Validation("credit note currency is required")
	}
	return &CreditNote{
		ID:          id,
		InvoiceID:   invoiceID,
		TenantID:    tenantID,
		Number:      number,
		AmountMinor: -creditMinor,
		Currency:    currency,
		Reason:      reason,
		CreatedAt:   now,
	}, nil
}
