package service

import (
	"testing"
	"time"
)

var prorationEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// inputFor builds a ProrationInput for a period of periodDays with the change
// happening elapsedDays into it (both day-granular, exact 24h days).
func inputFor(periodDays, elapsedDays int, oldUnit, newUnit int64) ProrationInput {
	start := prorationEpoch
	return ProrationInput{
		OldUnitMinor: oldUnit,
		NewUnitMinor: newUnit,
		Currency:     "USD",
		PeriodStart:  start,
		PeriodEnd:    start.Add(time.Duration(periodDays) * 24 * time.Hour),
		Now:          start.Add(time.Duration(elapsedDays) * 24 * time.Hour),
	}
}

// TestProrationImmediateMathTable proves the immediate proration is exact integer
// arithmetic: the remaining-days fraction of the price difference, truncated
// toward zero, with the boundary cases (first day, last day, equal price,
// downgrade credit, out-of-range clamps) all integer-exact.
func TestProrationImmediateMathTable(t *testing.T) {
	cases := []struct {
		name        string
		periodDays  int
		elapsedDays int
		oldUnit     int64
		newUnit     int64
		want        int64
	}{
		{"mid-period upgrade bills half", 30, 15, 1000, 2000, 500},
		{"first day bills the full difference", 30, 0, 1000, 2000, 1000},
		{"last day bills nothing", 30, 30, 1000, 2000, 0},
		{"equal price bills nothing", 30, 10, 1500, 1500, 0},
		{"downgrade is a proportional credit", 30, 15, 2000, 1000, -500},
		{"truncates toward zero, never over-bills", 30, 20, 1000, 2000, 333},
		{"zero-length period bills nothing", 0, 0, 1000, 2000, 0},
		{"change after period end clamps to zero", 30, 40, 1000, 2000, 0},
		{"change before period start bills the full difference", 30, -5, 1000, 2000, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := inputFor(tc.periodDays, tc.elapsedDays, tc.oldUnit, tc.newUnit)
			if got := proratedDifference(in); got != tc.want {
				t.Fatalf("proratedDifference = %d, want %d", got, tc.want)
			}
			// The immediate strategy bills exactly that, non-deferred.
			out := ImmediateProration{}.Compute(in)
			if out.AmountMinor != tc.want || out.Deferred {
				t.Fatalf("immediate outcome = %+v, want {AmountMinor:%d Deferred:false}", out, tc.want)
			}
		})
	}
}

// TestProrationStrategyNoneAndCreditNextInvoice proves strategy selection is
// honored: `none` bills nothing, `credit_next_invoice` computes the same amount
// as immediate but defers it, and an empty/unknown name defaults to immediate.
func TestProrationStrategyNoneAndCreditNextInvoice(t *testing.T) {
	in := inputFor(30, 15, 1000, 2000) // amount = 500
	const wantAmount = 500

	none := NewProrationStrategy(ProrationNone)
	if none.Name() != ProrationNone {
		t.Fatalf("none name = %q", none.Name())
	}
	if out := none.Compute(in); out.AmountMinor != 0 || out.Deferred {
		t.Fatalf("none outcome = %+v, want zero non-deferred", out)
	}

	credit := NewProrationStrategy(ProrationCreditNextInvoice)
	if credit.Name() != ProrationCreditNextInvoice {
		t.Fatalf("credit name = %q", credit.Name())
	}
	out := credit.Compute(in)
	if out.AmountMinor != wantAmount || !out.Deferred {
		t.Fatalf("credit_next_invoice outcome = %+v, want {AmountMinor:%d Deferred:true}", out, wantAmount)
	}

	// Empty and unknown names default to immediate_prorated.
	for _, name := range []string{"", "something_unknown", ProrationImmediate} {
		s := NewProrationStrategy(name)
		if s.Name() != ProrationImmediate {
			t.Fatalf("NewProrationStrategy(%q).Name() = %q, want %q", name, s.Name(), ProrationImmediate)
		}
		if got := s.Compute(in); got.AmountMinor != wantAmount || got.Deferred {
			t.Fatalf("default outcome for %q = %+v, want immediate %d", name, got, wantAmount)
		}
	}
}
