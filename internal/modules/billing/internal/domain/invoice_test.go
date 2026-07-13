package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestInvoiceLifecycleTransitionTable exhaustively checks every (status, event)
// pair against the allowed set draft→open→paid|void|uncollectible.
func TestInvoiceLifecycleTransitionTable(t *testing.T) {
	allowed := map[Status]map[Event]Status{
		StatusDraft: {EventOpen: StatusOpen, EventVoid: StatusVoid},
		StatusOpen: {
			EventPay:               StatusPaid,
			EventVoid:              StatusVoid,
			EventMarkUncollectible: StatusUncollectible,
		},
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, from := range AllStatuses() {
		for _, ev := range AllEvents() {
			want, ok := allowed[from][ev]
			inv := &Invoice{Status: from}
			err := inv.Apply(ev, now)
			if ok {
				if err != nil {
					t.Errorf("Apply(%s, %s): unexpected error %v", from, ev, err)
					continue
				}
				if inv.Status != want {
					t.Errorf("Apply(%s, %s): status = %s, want %s", from, ev, inv.Status, want)
				}
			} else {
				if err == nil {
					t.Errorf("Apply(%s, %s): expected error, got status %s", from, ev, inv.Status)
				}
				if inv.Status != from {
					t.Errorf("Apply(%s, %s): status changed to %s on illegal transition", from, ev, inv.Status)
				}
			}
		}
	}
}

func TestOpenStampsIssuedAt(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	inv := &Invoice{Status: StatusDraft}
	if err := inv.Apply(EventOpen, now); err != nil {
		t.Fatalf("open: %v", err)
	}
	if inv.IssuedAt == nil || !inv.IssuedAt.Equal(now) {
		t.Fatalf("IssuedAt = %v, want %v", inv.IssuedAt, now)
	}
}

// TestTotalsSumLineItemsInMinorUnits asserts totals are pure integer minor-unit
// arithmetic: line amount = unit × quantity, subtotal = Σ amounts, total =
// subtotal + tax. No floats are used anywhere.
func TestTotalsSumLineItemsInMinorUnits(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lines := []LineItem{
		{Kind: LineKindPlan, Key: "pro", Version: 1, UnitPriceMinor: 1000, Quantity: 1, Currency: "USD"},
		{Kind: LineKindAddon, Key: "seats", Version: 2, UnitPriceMinor: 550, Quantity: 3, Currency: "USD"},
		{Kind: LineKindAddon, Key: "storage", Version: 1, UnitPriceMinor: 99, Quantity: 7, Currency: "USD"},
	}
	// 1000*1 + 550*3 + 99*7 = 1000 + 1650 + 693 = 3343.
	const wantSubtotal int64 = 3343
	if got := Subtotal(lines); got != wantSubtotal {
		t.Fatalf("Subtotal = %d, want %d", got, wantSubtotal)
	}

	// Tax of 267 minor units (already rounded to integer minor units by a
	// calculator) → total 3610. The domain never divides or uses floats.
	const taxMinor int64 = 267
	inv, err := NewInvoice(uuid.New(), uuid.New(), uuid.New(), 1, "USD", lines, taxMinor, now)
	if err != nil {
		t.Fatalf("NewInvoice: %v", err)
	}
	if inv.SubtotalMinor != wantSubtotal {
		t.Errorf("SubtotalMinor = %d, want %d", inv.SubtotalMinor, wantSubtotal)
	}
	if inv.TaxMinor != taxMinor {
		t.Errorf("TaxMinor = %d, want %d", inv.TaxMinor, taxMinor)
	}
	if want := wantSubtotal + taxMinor; inv.TotalMinor != want {
		t.Errorf("TotalMinor = %d, want %d", inv.TotalMinor, want)
	}

	// Per-line amount is exact integer multiplication.
	if got := lines[1].Amount(); got != 1650 {
		t.Errorf("line Amount = %d, want 1650", got)
	}
}

func TestNewInvoiceRejectsMixedCurrencyAndEmpty(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := NewInvoice(uuid.New(), uuid.New(), uuid.New(), 1, "USD", nil, 0, now); err == nil {
		t.Error("expected error for empty lines")
	}
	mixed := []LineItem{
		{Kind: LineKindPlan, Key: "pro", Version: 1, UnitPriceMinor: 1000, Quantity: 1, Currency: "EUR"},
	}
	if _, err := NewInvoice(uuid.New(), uuid.New(), uuid.New(), 1, "USD", mixed, 0, now); err == nil {
		t.Error("expected error for currency mismatch")
	}
}

func TestNewCreditNoteNegatesAmount(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	invID := uuid.New()
	cn, err := NewCreditNote(uuid.New(), invID, uuid.New(), 1, 3610, "USD", "refund", now)
	if err != nil {
		t.Fatalf("NewCreditNote: %v", err)
	}
	if cn.AmountMinor != -3610 {
		t.Errorf("AmountMinor = %d, want -3610", cn.AmountMinor)
	}
	if cn.InvoiceID != invID {
		t.Errorf("InvoiceID = %v, want %v", cn.InvoiceID, invID)
	}
	if _, err := NewCreditNote(uuid.New(), invID, uuid.New(), 2, 100, "USD", "", now); err == nil {
		t.Error("expected error for missing reason")
	}
	if _, err := NewCreditNote(uuid.New(), invID, uuid.New(), 3, 0, "USD", "x", now); err == nil {
		t.Error("expected error for non-positive amount")
	}
}
