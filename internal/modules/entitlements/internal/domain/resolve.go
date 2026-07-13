package domain

import (
	"encoding/json"
	"time"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// Entitlement source layers, in precedence order (plan < addon < override).
const (
	SourceDefault  = "default"
	SourcePlan     = "plan"
	SourceAddon    = "addon"
	SourceOverride = "override"
)

// Delta kinds an addon applies (mirrors catalog's addon deltas).
const (
	DeltaLimit    = "limit_delta"
	DeltaOverride = "value_override"
)

// UnknownPolicy decides how resolution treats a feature key that is referenced
// (by a plan grant, addon delta, or override) but absent from the registry.
type UnknownPolicy string

// Unknown-feature policies.
const (
	// UnknownDeny drops references to keys absent from the registry (default).
	UnknownDeny UnknownPolicy = "default_deny"
	// UnknownAllow keeps them, trusting the referenced value.
	UnknownAllow UnknownPolicy = "default_allow"
)

// AppliedDelta is an addon delta paired with the attached quantity. A limit
// delta contributes Amount*Quantity to the base; a value override sets Value
// (quantity is ignored for overrides).
type AppliedDelta struct {
	FeatureKey string
	Kind       string
	Amount     int64
	Value      any
	Quantity   int
}

// Resolved is a feature's effective value and the layer that produced it.
// ExpiresAt is set only when the winning layer is a time-bound override, so
// clients can see when the effective value will revert.
type Resolved struct {
	Value     any
	Source    string
	ExpiresAt *time.Time
}

// ResolveInput is everything the pure resolver composes.
type ResolveInput struct {
	// Features is the active registry, keyed by feature key. Archived features
	// are omitted, so they stop granting.
	Features map[string]Feature
	// PlanGrants is the pinned plan version's base feature grants.
	PlanGrants map[string]any
	// AddonDeltas are the deltas of attached addon versions, with quantities.
	AddonDeltas []AppliedDelta
	// Overrides are the tenant's live (non-expired) overrides, keyed by feature.
	Overrides map[string]any
	// OverrideExpiry carries the expires_at of a time-bound override, keyed by
	// feature. It is optional and only informational: it does not affect which
	// value wins, only the ExpiresAt surfaced on the resolved entry.
	OverrideExpiry map[string]*time.Time
	// Policy governs keys absent from the registry.
	Policy UnknownPolicy
}

// Resolve composes the effective entitlement set with precedence
// plan < addon < override, starting from each active feature's default. It
// returns a validation error when a layer supplies a value or delta that is
// illegal for the target feature's type (e.g. a limit delta on a boolean).
func Resolve(in ResolveInput) (map[string]Resolved, error) {
	out := make(map[string]Resolved, len(in.Features))

	// Layer 0: registry defaults for every active feature.
	for key, f := range in.Features {
		out[key] = Resolved{Value: f.DefaultValue, Source: SourceDefault}
	}

	// Layer 1: plan-version base grants.
	for key, val := range in.PlanGrants {
		f, known := in.Features[key]
		if !known {
			if in.Policy == UnknownAllow {
				out[key] = Resolved{Value: val, Source: SourcePlan}
			}
			continue
		}
		if err := f.ValidateValue(val); err != nil {
			return nil, err
		}
		out[key] = Resolved{Value: val, Source: SourcePlan}
	}

	// Layer 2: addon deltas (× quantity for limit deltas).
	for _, d := range in.AddonDeltas {
		f, known := in.Features[d.FeatureKey]
		if !known && in.Policy == UnknownDeny {
			continue
		}
		switch d.Kind {
		case DeltaLimit:
			if known && f.Type != FeatureLimit {
				return nil, apperr.Validation("limit_delta cannot apply to non-limit feature " + d.FeatureKey)
			}
			qty := d.Quantity
			if qty < 1 {
				qty = 1
			}
			base := int64(0)
			if cur, ok := out[d.FeatureKey]; ok {
				if n, ok := toInt64(cur.Value); ok {
					base = n
				}
			}
			out[d.FeatureKey] = Resolved{Value: base + d.Amount*int64(qty), Source: SourceAddon}
		case DeltaOverride:
			if known {
				if err := f.ValidateValue(d.Value); err != nil {
					return nil, err
				}
			}
			out[d.FeatureKey] = Resolved{Value: d.Value, Source: SourceAddon}
		default:
			return nil, apperr.Validation("unknown delta kind: " + d.Kind)
		}
	}

	// Layer 3: tenant overrides win outright.
	for key, val := range in.Overrides {
		f, known := in.Features[key]
		if !known {
			if in.Policy == UnknownAllow {
				out[key] = Resolved{Value: val, Source: SourceOverride, ExpiresAt: in.OverrideExpiry[key]}
			}
			continue
		}
		if err := f.ValidateValue(val); err != nil {
			return nil, err
		}
		out[key] = Resolved{Value: val, Source: SourceOverride, ExpiresAt: in.OverrideExpiry[key]}
	}

	return out, nil
}

// toInt64 coerces a JSON-decoded numeric value to int64. Limits are integers,
// but jsonb round-trips them as float64, so both are accepted.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		// Reject non-integral floats: a limit must be a whole number.
		if n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}
