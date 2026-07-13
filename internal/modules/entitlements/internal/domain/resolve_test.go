package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// registry builds an active-feature map from the given features.
func registry(fs ...*Feature) map[string]Feature {
	m := make(map[string]Feature, len(fs))
	for _, f := range fs {
		m[f.Key] = *f
	}
	return m
}

func mustFeature(t *testing.T, key string, typ FeatureType, def any, meta map[string]any) *Feature {
	t.Helper()
	f, err := NewFeature(uuid.New(), key, typ, def, "", "", "", meta, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("NewFeature(%s): %v", key, err)
	}
	return f
}

func TestResolutionPrecedenceTable(t *testing.T) {
	seats := mustFeature(t, "seats", FeatureLimit, int64(10), nil)
	sso := mustFeature(t, "sso", FeatureBoolean, false, nil)
	theme := mustFeature(t, "theme", FeatureConfig, "light", nil)
	tier := mustFeature(t, "tier", FeatureEnum, "silver", map[string]any{"allowed_values": []any{"silver", "gold", "platinum"}})
	reg := registry(seats, sso, theme, tier)

	tests := []struct {
		name       string
		grants     map[string]any
		deltas     []AppliedDelta
		overrides  map[string]any
		wantValue  any
		wantSource string
		key        string
	}{
		{
			name: "plan only (limit)", key: "seats",
			grants:    map[string]any{"seats": float64(10)},
			wantValue: float64(10), wantSource: SourcePlan,
		},
		{
			name: "default when no layer touches it (boolean)", key: "sso",
			wantValue: false, wantSource: SourceDefault,
		},
		{
			name: "plan grants boolean", key: "sso",
			grants:    map[string]any{"sso": true},
			wantValue: true, wantSource: SourcePlan,
		},
		{
			name: "plan+addon (limit delta lifts base)", key: "seats",
			grants:    map[string]any{"seats": float64(10)},
			deltas:    []AppliedDelta{{FeatureKey: "seats", Kind: DeltaLimit, Amount: 5, Quantity: 1}},
			wantValue: int64(15), wantSource: SourceAddon,
		},
		{
			name: "plan+override (override wins)", key: "seats",
			grants:    map[string]any{"seats": float64(10)},
			overrides: map[string]any{"seats": float64(99)},
			wantValue: float64(99), wantSource: SourceOverride,
		},
		{
			name: "all three: override beats plan+addon", key: "seats",
			grants:    map[string]any{"seats": float64(10)},
			deltas:    []AppliedDelta{{FeatureKey: "seats", Kind: DeltaLimit, Amount: 5, Quantity: 2}},
			overrides: map[string]any{"seats": float64(50)},
			wantValue: float64(50), wantSource: SourceOverride,
		},
		{
			name: "config value override via addon", key: "theme",
			grants:    map[string]any{"theme": "light"},
			deltas:    []AppliedDelta{{FeatureKey: "theme", Kind: DeltaOverride, Value: "dark"}},
			wantValue: "dark", wantSource: SourceAddon,
		},
		{
			name: "enum override wins", key: "tier",
			grants:    map[string]any{"tier": "gold"},
			overrides: map[string]any{"tier": "platinum"},
			wantValue: "platinum", wantSource: SourceOverride,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(ResolveInput{
				Features: reg, PlanGrants: tc.grants, AddonDeltas: tc.deltas, Overrides: tc.overrides, Policy: UnknownDeny,
			})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			r, ok := got[tc.key]
			if !ok {
				t.Fatalf("key %s absent from resolved set", tc.key)
			}
			if r.Source != tc.wantSource {
				t.Errorf("source = %s, want %s", r.Source, tc.wantSource)
			}
			if !valuesEqual(r.Value, tc.wantValue) {
				t.Errorf("value = %#v, want %#v", r.Value, tc.wantValue)
			}
		})
	}
}

func TestAddonLimitDeltaTimesQuantity(t *testing.T) {
	seats := mustFeature(t, "seats", FeatureLimit, int64(10), nil)
	reg := registry(seats)

	cases := []struct {
		name     string
		amount   int64
		quantity int
		want     int64
	}{
		{"base 10 + delta 10 x1 => 20", 10, 1, 20},
		{"base 10 + delta 10 x2 => 30 (quantity multiplies)", 10, 2, 30},
		{"base 10 + delta 5 x2 => 20", 5, 2, 20},
		{"base 10 + delta 5 x1 => 15 (quantity matters)", 5, 1, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(ResolveInput{
				Features:    reg,
				PlanGrants:  map[string]any{"seats": float64(10)},
				AddonDeltas: []AppliedDelta{{FeatureKey: "seats", Kind: DeltaLimit, Amount: tc.amount, Quantity: tc.quantity}},
				Policy:      UnknownDeny,
			})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			n, ok := toInt64(got["seats"].Value)
			if !ok {
				t.Fatalf("seats value not integer: %#v", got["seats"].Value)
			}
			if n != tc.want {
				t.Errorf("seats = %d, want %d", n, tc.want)
			}
			if got["seats"].Source != SourceAddon {
				t.Errorf("source = %s, want addon", got["seats"].Source)
			}
		})
	}
}

func TestUnknownFeaturePolicyDenyAndAllow(t *testing.T) {
	known := mustFeature(t, "seats", FeatureLimit, int64(10), nil)
	reg := registry(known)
	// "beta_x" is granted by the plan but absent from the registry.
	grants := map[string]any{"seats": float64(10), "beta_x": true}

	denied, err := Resolve(ResolveInput{Features: reg, PlanGrants: grants, Policy: UnknownDeny})
	if err != nil {
		t.Fatalf("deny resolve: %v", err)
	}
	if _, ok := denied["beta_x"]; ok {
		t.Errorf("deny policy leaked unknown feature into the set")
	}
	if _, ok := denied["seats"]; !ok {
		t.Errorf("deny policy dropped a known feature")
	}

	allowed, err := Resolve(ResolveInput{Features: reg, PlanGrants: grants, Policy: UnknownAllow})
	if err != nil {
		t.Fatalf("allow resolve: %v", err)
	}
	r, ok := allowed["beta_x"]
	if !ok {
		t.Fatalf("allow policy dropped the unknown feature")
	}
	if r.Value != true || r.Source != SourcePlan {
		t.Errorf("beta_x = %#v/%s, want true/plan", r.Value, r.Source)
	}
}

func TestFeatureTypeValidation(t *testing.T) {
	boolean := mustFeature(t, "sso", FeatureBoolean, false, nil)
	limit := mustFeature(t, "seats", FeatureLimit, int64(10), nil)
	config := mustFeature(t, "theme", FeatureConfig, "light", nil)
	tier := mustFeature(t, "tier", FeatureEnum, "silver", map[string]any{"allowed_values": []any{"silver", "gold"}})
	reg := registry(boolean, limit, config, tier)

	// A limit_delta on a boolean feature is invalid.
	if _, err := Resolve(ResolveInput{
		Features:    reg,
		AddonDeltas: []AppliedDelta{{FeatureKey: "sso", Kind: DeltaLimit, Amount: 1, Quantity: 1}},
		Policy:      UnknownDeny,
	}); err == nil {
		t.Errorf("expected error for limit_delta on boolean feature")
	}

	// A limit_delta on a config feature is invalid.
	if _, err := Resolve(ResolveInput{
		Features:    reg,
		AddonDeltas: []AppliedDelta{{FeatureKey: "theme", Kind: DeltaLimit, Amount: 1, Quantity: 1}},
		Policy:      UnknownDeny,
	}); err == nil {
		t.Errorf("expected error for limit_delta on config feature")
	}

	// An enum value outside the allowed set is rejected.
	if _, err := Resolve(ResolveInput{
		Features:  reg,
		Overrides: map[string]any{"tier": "diamond"},
		Policy:    UnknownDeny,
	}); err == nil {
		t.Errorf("expected error for enum value outside allowed set")
	}

	// An enum value inside the allowed set is accepted.
	if _, err := Resolve(ResolveInput{
		Features:  reg,
		Overrides: map[string]any{"tier": "gold"},
		Policy:    UnknownDeny,
	}); err != nil {
		t.Errorf("unexpected error for allowed enum value: %v", err)
	}

	// A non-boolean value on a boolean feature is rejected.
	if _, err := Resolve(ResolveInput{
		Features:   reg,
		PlanGrants: map[string]any{"sso": "yes"},
		Policy:     UnknownDeny,
	}); err == nil {
		t.Errorf("expected error for string value on boolean feature")
	}
}

func TestArchivedFeatureOmittedFromRegistry(t *testing.T) {
	// Archived features are excluded from the registry map the resolver sees, so
	// they stop granting.
	active := mustFeature(t, "seats", FeatureLimit, int64(10), nil)
	reg := registry(active) // an archived feature simply is not added here

	got, err := Resolve(ResolveInput{
		Features:   reg,
		PlanGrants: map[string]any{"seats": float64(10), "legacy": true},
		Policy:     UnknownDeny,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, ok := got["legacy"]; ok {
		t.Errorf("archived/unknown feature granted under deny policy")
	}
}

// valuesEqual compares two resolved values, coercing numbers so float64(10) and
// int64(10) compare equal (jsonb round-trips integers as float64).
func valuesEqual(a, b any) bool {
	if an, aok := toInt64(a); aok {
		if bn, bok := toInt64(b); bok {
			return an == bn
		}
	}
	return a == b
}
