package fakeprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
)

// TestFakeProviderProgrammableFailuresAndCallRecording is the fake harness'
// own contract: it auto-succeeds by default, honors one-shot and permanent
// programmed declines, surfaces a programmed transport error, and records every
// Charge/CreateCustomer/AttachPaymentMethod call for later assertion.
func TestFakeProviderProgrammableFailuresAndCallRecording(t *testing.T) {
	ctx := context.Background()
	p := New()
	inv := uuid.New()
	tenant := uuid.New()

	// Default: charges succeed.
	res, err := p.Charge(ctx, service.ChargeRequest{IdempotencyKey: "k1", TenantID: tenant, InvoiceID: inv, AmountMinor: 1000, Currency: "USD"})
	if err != nil {
		t.Fatalf("k1 charge: unexpected error %v", err)
	}
	if !res.Success {
		t.Fatal("k1 charge: expected success by default")
	}

	// One-shot programmed decline: the next charge fails, the one after succeeds.
	p.FailNextCharges(1, "insufficient_funds")
	res, err = p.Charge(ctx, service.ChargeRequest{IdempotencyKey: "k2", InvoiceID: inv, AmountMinor: 1000, Currency: "USD"})
	if err != nil {
		t.Fatalf("k2 charge: unexpected error %v", err)
	}
	if res.Success {
		t.Fatal("k2 charge: expected a programmed decline")
	}
	if res.FailureReason != "insufficient_funds" {
		t.Fatalf("k2 reason = %q, want insufficient_funds", res.FailureReason)
	}
	res, err = p.Charge(ctx, service.ChargeRequest{IdempotencyKey: "k3", InvoiceID: inv, AmountMinor: 1000, Currency: "USD"})
	if err != nil || !res.Success {
		t.Fatalf("k3 charge: one-shot decline leaked, res=%+v err=%v", res, err)
	}

	// Programmed transport fault (returned as an error, not a decline).
	boom := errors.New("gateway timeout")
	p.FailChargeWithError(boom)
	if _, err := p.Charge(ctx, service.ChargeRequest{IdempotencyKey: "k4", InvoiceID: inv, AmountMinor: 1000, Currency: "USD"}); !errors.Is(err, boom) {
		t.Fatalf("k4 charge: err = %v, want %v", err, boom)
	}
	p.FailChargeWithError(nil)

	// Call recording: every Charge was captured, in order, with its key.
	calls := p.Charges()
	if len(calls) != 4 {
		t.Fatalf("recorded %d charges, want 4", len(calls))
	}
	if calls[0].IdempotencyKey != "k1" || calls[3].IdempotencyKey != "k4" {
		t.Fatalf("recorded keys out of order: %+v", calls)
	}
	if calls[0].AmountMinor != 1000 || calls[0].Currency != "USD" || calls[0].InvoiceID != inv {
		t.Fatalf("charge amount/currency/invoice not recorded: %+v", calls[0])
	}
	if p.ChargeCount() != 4 {
		t.Fatalf("ChargeCount = %d, want 4", p.ChargeCount())
	}

	// Permanent decline mode.
	p.FailAllCharges("gateway_down")
	for i := 0; i < 3; i++ {
		r, err := p.Charge(ctx, service.ChargeRequest{IdempotencyKey: "x", InvoiceID: inv})
		if err != nil {
			t.Fatalf("permanent decline charge %d: unexpected error %v", i, err)
		}
		if r.Success {
			t.Fatalf("permanent decline charge %d: expected decline", i)
		}
	}

	// Customer + payment-method recording; tokens echoed, never a PAN synthesized.
	cust, err := p.CreateCustomer(ctx, service.CustomerRequest{TenantID: tenant, Email: "a@b.c"})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	ref, err := p.AttachPaymentMethod(ctx, service.AttachPaymentMethodRequest{CustomerRef: cust.ID, Token: "pm_visa_123", Brand: "visa", Last4: "4242"})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if ref.Token != "pm_visa_123" {
		t.Fatalf("attach echoed token %q, want pm_visa_123", ref.Token)
	}
	if got := p.Customers(); len(got) != 1 || got[0].TenantID != tenant {
		t.Fatalf("customers recording = %+v", got)
	}
	if got := p.Attaches(); len(got) != 1 || got[0].Token != "pm_visa_123" {
		t.Fatalf("attaches recording = %+v", got)
	}

	// The construction-time option is equivalent to programming after the fact.
	p2 := NewWith(WithChargeFailure(-1, "always"))
	if r, _ := p2.Charge(ctx, service.ChargeRequest{IdempotencyKey: "y"}); r.Success {
		t.Fatal("WithChargeFailure(-1) should decline every charge")
	}
}
