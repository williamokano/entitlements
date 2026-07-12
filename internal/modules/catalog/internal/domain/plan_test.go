package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func draftWithPrice(t *testing.T) *PlanVersion {
	t.Helper()
	v := NewDraftVersion(uuid.New(), uuid.New(), 1, now)
	if err := v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: 1000}}, TrialConfig{}, 0, nil); err != nil {
		t.Fatalf("SetContent: %v", err)
	}
	return v
}

func TestPublishFreezesVersionMutationsRejected(t *testing.T) {
	v := draftWithPrice(t)
	if err := v.Publish(now); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !v.Published() {
		t.Fatal("version not marked published")
	}
	// Editing a published version is rejected with a conflict.
	err := v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: 2000}}, TrialConfig{}, 0, nil)
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("SetContent on published version kind = %v, want conflict", apperr.KindOf(err))
	}
	// Publishing again is rejected too.
	if apperr.KindOf(v.Publish(now)) != apperr.KindConflict {
		t.Fatal("double publish not rejected")
	}
	// The pinned price is unchanged.
	if v.Prices[0].AmountMinor != 1000 {
		t.Fatalf("published price mutated to %d", v.Prices[0].AmountMinor)
	}
}

func TestPlanLifecycleTransitionTable(t *testing.T) {
	p, err := NewPlan(uuid.New(), "pro", "Pro", now)
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}
	if p.Status != PlanDraft {
		t.Fatalf("new plan status = %s, want draft", p.Status)
	}
	if err := p.Activate(); err != nil {
		t.Fatalf("draft→active: %v", err)
	}
	// active→draft is illegal.
	if apperr.KindOf(p.Activate()) != apperr.KindConflict {
		t.Fatal("active→active/illegal move not rejected")
	}
	if err := p.Archive(); err != nil {
		t.Fatalf("active→archived: %v", err)
	}
	// archived is terminal.
	if apperr.KindOf(p.Archive()) != apperr.KindConflict {
		t.Fatal("archived→archived not rejected")
	}
}

func TestPriceValidationMinorUnitsAndCurrency(t *testing.T) {
	v := NewDraftVersion(uuid.New(), uuid.New(), 1, now)

	// Missing currency.
	if apperr.KindOf(v.SetContent("", []Price{{Cycle: CycleMonthly, AmountMinor: 100}}, TrialConfig{}, 0, nil)) != apperr.KindValidation {
		t.Fatal("empty currency accepted")
	}
	// Bad currency shape.
	if apperr.KindOf(v.SetContent("usd", []Price{{Cycle: CycleMonthly, AmountMinor: 100}}, TrialConfig{}, 0, nil)) != apperr.KindValidation {
		t.Fatal("lowercase currency accepted")
	}
	// Negative amount.
	if apperr.KindOf(v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: -1}}, TrialConfig{}, 0, nil)) != apperr.KindValidation {
		t.Fatal("negative amount accepted")
	}
	// No prices.
	if apperr.KindOf(v.SetContent("USD", nil, TrialConfig{}, 0, nil)) != apperr.KindValidation {
		t.Fatal("empty prices accepted")
	}
	// Duplicate cycle.
	dup := []Price{{Cycle: CycleMonthly, AmountMinor: 100}, {Cycle: CycleMonthly, AmountMinor: 200}}
	if apperr.KindOf(v.SetContent("USD", dup, TrialConfig{}, 0, nil)) != apperr.KindValidation {
		t.Fatal("duplicate cycle accepted")
	}
	// Valid.
	if err := v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: 100}, {Cycle: CycleAnnual, AmountMinor: 1000}}, TrialConfig{}, 0, nil); err != nil {
		t.Fatalf("valid prices rejected: %v", err)
	}
}

func TestTrialAndGraceConfigValidation(t *testing.T) {
	v := NewDraftVersion(uuid.New(), uuid.New(), 1, now)
	base := []Price{{Cycle: CycleMonthly, AmountMinor: 100}}

	// Negative grace.
	if apperr.KindOf(v.SetContent("USD", base, TrialConfig{}, -1, nil)) != apperr.KindValidation {
		t.Fatal("negative grace accepted")
	}
	// Trial enabled with zero days.
	if apperr.KindOf(v.SetContent("USD", base, TrialConfig{Enabled: true, Days: 0}, 0, nil)) != apperr.KindValidation {
		t.Fatal("enabled trial with 0 days accepted")
	}
	// Trial disabled: stray days are zeroed, not an error.
	if err := v.SetContent("USD", base, TrialConfig{Enabled: false, Days: 14, CardRequired: true}, 0, nil); err != nil {
		t.Fatalf("disabled trial with days rejected: %v", err)
	}
	if v.Trial.Days != 0 || v.Trial.CardRequired {
		t.Fatalf("disabled trial not normalized: %+v", v.Trial)
	}
}
