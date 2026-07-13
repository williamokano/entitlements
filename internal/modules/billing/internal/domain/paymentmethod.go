package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// PaymentMethod is a stored, tokenized payment instrument for a tenant. It holds
// only opaque provider references — a customer ref and a payment-method token —
// plus non-sensitive display metadata (brand, last four). A raw card number
// (PAN) is never accepted or stored: tokenization happens at the gateway and only
// the token crosses into this system.
type PaymentMethod struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	CustomerRef string
	Token       string
	Brand       string
	Last4       string
	CreatedAt   time.Time
}

// NewPaymentMethod builds a PaymentMethod, rejecting anything that looks like a
// raw card number in the token or customer reference. A PAN must never reach
// storage — callers must pass a provider token reference (e.g. "pm_..." /
// "tok_...").
func NewPaymentMethod(id, tenantID uuid.UUID, customerRef, token, brand, last4 string, now time.Time) (*PaymentMethod, error) {
	if strings.TrimSpace(token) == "" {
		return nil, apperr.Validation("payment method token is required")
	}
	if looksLikePAN(token) {
		return nil, apperr.Validation("refusing to store a raw card number; store a provider token reference instead")
	}
	if looksLikePAN(customerRef) {
		return nil, apperr.Validation("refusing to store a raw card number in the customer reference")
	}
	if l := strings.TrimSpace(last4); l != "" && !isDigits(l) {
		return nil, apperr.Validation("last4 must be numeric")
	}
	return &PaymentMethod{
		ID:          id,
		TenantID:    tenantID,
		CustomerRef: customerRef,
		Token:       token,
		Brand:       brand,
		Last4:       last4,
		CreatedAt:   now,
	}, nil
}

// looksLikePAN reports whether s, once separators are stripped, is a bare run of
// 12–19 digits — the shape of a real card number. Provider tokens carry a prefix
// or non-digit characters, so they pass; a raw PAN is rejected.
func looksLikePAN(s string) bool {
	stripped := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(s))
	if len(stripped) < 12 || len(stripped) > 19 {
		return false
	}
	return isDigits(stripped)
}

// isDigits reports whether s is non-empty and all ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
