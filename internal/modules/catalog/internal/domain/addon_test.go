package domain

import (
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

func addonVersion(t *testing.T, quantityAllowed bool, compatible []string, deltas []Delta) *AddonVersion {
	t.Helper()
	v := NewDraftAddonVersion(uuid.New(), uuid.New(), 1, now)
	if err := v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: 500}}, quantityAllowed, compatible, deltas); err != nil {
		t.Fatalf("SetContent: %v", err)
	}
	return v
}

func TestAddonCompatibilityHelperAcceptsAndRejects(t *testing.T) {
	v := addonVersion(t, false, []string{"pro", "enterprise"}, nil)

	if !v.CompatibleWith("pro") {
		t.Fatal("pro should be compatible")
	}
	if v.CompatibleWith("starter") {
		t.Fatal("starter should be incompatible")
	}
	if err := v.EnsureCompatible("pro"); err != nil {
		t.Fatalf("EnsureCompatible(pro) = %v, want nil", err)
	}
	if apperr.KindOf(v.EnsureCompatible("starter")) != apperr.KindValidation {
		t.Fatal("EnsureCompatible(starter) should be a validation error")
	}
}

func TestAddonDeltaValidation(t *testing.T) {
	// Valid: a limit delta and a value override on distinct keys.
	good := []Delta{
		{FeatureKey: "max_projects", Kind: DeltaLimit, Amount: 10},
		{FeatureKey: "sso", Kind: DeltaOverride, Value: true},
	}
	if _, err := trySetDeltas(t, good); err != nil {
		t.Fatalf("valid deltas rejected: %v", err)
	}

	cases := map[string][]Delta{
		"empty key":         {{FeatureKey: "", Kind: DeltaLimit, Amount: 1}},
		"limit with value":  {{FeatureKey: "x", Kind: DeltaLimit, Amount: 1, Value: 5}},
		"override no value": {{FeatureKey: "x", Kind: DeltaOverride}},
		"override w/amount": {{FeatureKey: "x", Kind: DeltaOverride, Amount: 1, Value: 2}},
		"bad kind":          {{FeatureKey: "x", Kind: "weird"}},
		"duplicate key": {
			{FeatureKey: "x", Kind: DeltaLimit, Amount: 1},
			{FeatureKey: "x", Kind: DeltaLimit, Amount: 2},
		},
	}
	for name, deltas := range cases {
		if _, err := trySetDeltas(t, deltas); apperr.KindOf(err) != apperr.KindValidation {
			t.Errorf("%s: kind = %v, want validation error", name, apperr.KindOf(err))
		}
	}
}

func TestQuantityFlagEnforcedAtValidation(t *testing.T) {
	off := addonVersion(t, false, []string{"pro"}, nil)
	if err := off.ValidateQuantity(1); err != nil {
		t.Fatalf("qty 1 rejected when flag off: %v", err)
	}
	if apperr.KindOf(off.ValidateQuantity(2)) != apperr.KindValidation {
		t.Fatal("qty 2 accepted when flag off")
	}
	if apperr.KindOf(off.ValidateQuantity(0)) != apperr.KindValidation {
		t.Fatal("qty 0 accepted")
	}

	on := addonVersion(t, true, []string{"pro"}, nil)
	if err := on.ValidateQuantity(3); err != nil {
		t.Fatalf("qty 3 rejected when flag on: %v", err)
	}
}

func TestPublishFreezesAddonVersion(t *testing.T) {
	v := addonVersion(t, true, []string{"pro"}, []Delta{{FeatureKey: "seats", Kind: DeltaLimit, Amount: 5}})
	if err := v.Publish(now); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if apperr.KindOf(v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: 999}}, true, []string{"pro"}, nil)) != apperr.KindConflict {
		t.Fatal("edit on published addon version not rejected")
	}
}

func trySetDeltas(t *testing.T, deltas []Delta) (*AddonVersion, error) {
	t.Helper()
	v := NewDraftAddonVersion(uuid.New(), uuid.New(), 1, now)
	err := v.SetContent("USD", []Price{{Cycle: CycleMonthly, AmountMinor: 500}}, false, []string{"pro"}, deltas)
	return v, err
}
