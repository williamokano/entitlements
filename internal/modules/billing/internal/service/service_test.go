package service

import (
	"context"
	"testing"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/domain"
)

// recordingTax wraps an inner calculator and records that it was invoked and
// with how many lines.
type recordingTax struct {
	inner  TaxCalculator
	called bool
	lines  int
}

func (r *recordingTax) Calculate(ctx context.Context, lines []domain.LineItem, currency string) (int64, error) {
	r.called = true
	r.lines = len(lines)
	return r.inner.Calculate(ctx, lines, currency)
}

// TestTaxCalculatorPortInvokedNoopDefaultZeroTax asserts that issuance invokes
// the TaxCalculator port and that the shipped default (NoopTaxCalculator)
// returns zero tax. computeTax is the single place issuance computes tax.
func TestTaxCalculatorPortInvokedNoopDefaultZeroTax(t *testing.T) {
	lines := []domain.LineItem{
		{Kind: domain.LineKindPlan, Key: "pro", Version: 1, UnitPriceMinor: 1000, Quantity: 1, Currency: "USD"},
	}
	rec := &recordingTax{inner: NoopTaxCalculator{}}
	svc := New(nil, nil, nil, nil, nil, rec, nil, nil)

	tax, err := svc.tax.Calculate(context.Background(), lines, "USD")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if !rec.called {
		t.Fatal("expected the tax calculator to be invoked")
	}
	if rec.lines != 1 {
		t.Fatalf("calculator saw %d lines, want 1", rec.lines)
	}
	if tax != 0 {
		t.Fatalf("noop tax = %d, want 0", tax)
	}
}

// TestNewDefaultsToNoopTaxCalculator asserts a nil calculator defaults to the
// no-op (zero tax), so the module is safe to wire without a tax provider.
func TestNewDefaultsToNoopTaxCalculator(t *testing.T) {
	svc := New(nil, nil, nil, nil, nil, nil, nil, nil)
	tax, err := svc.tax.Calculate(context.Background(), nil, "USD")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if tax != 0 {
		t.Fatalf("default tax = %d, want 0", tax)
	}
}
