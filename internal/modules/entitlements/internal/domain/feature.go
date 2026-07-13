// Package domain is the entitlements module's pure core: the Feature aggregate
// (the dynamic feature registry) and the resolution function that composes plan
// grants, addon deltas, and tenant overrides into a tenant's effective
// entitlements. It imports only the standard library, uuid, and platform/apperr.
package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// FeatureType classifies what a feature grants.
type FeatureType string

// Feature types.
const (
	// FeatureBoolean is an on/off capability.
	FeatureBoolean FeatureType = "boolean"
	// FeatureLimit is an integer quota (extendable by addon limit deltas).
	FeatureLimit FeatureType = "limit"
	// FeatureConfig is a free-form configuration value (any JSON).
	FeatureConfig FeatureType = "config"
	// FeatureEnum is a value constrained to an allowed set of strings.
	FeatureEnum FeatureType = "enum"
)

// Valid reports whether t is a known feature type.
func (t FeatureType) Valid() bool {
	switch t {
	case FeatureBoolean, FeatureLimit, FeatureConfig, FeatureEnum:
		return true
	default:
		return false
	}
}

// Limit behaviors for a limit feature: warn (soft) or block (hard).
const (
	LimitSoft = "soft"
	LimitHard = "hard"
)

// Reset periods for a limit feature's usage counter.
const (
	ResetBillingCycle = "billing_cycle"
	ResetMonthly      = "monthly"
	ResetNever        = "never"
)

// metadataAllowedValues is the metadata key under which an enum feature's allowed
// set is stored. Keeping it in metadata avoids a dedicated column while still
// letting resolution constrain enum values.
const metadataAllowedValues = "allowed_values"

// Feature is a registry entry: a named, typed capability with a default value.
// Features are archived (Active=false), never deleted, so historical
// entitlements stay coherent.
type Feature struct {
	ID            uuid.UUID
	Key           string
	Type          FeatureType
	DefaultValue  any
	Description   string
	LimitBehavior string
	ResetPeriod   string
	Metadata      map[string]any
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewFeature builds and validates a new active feature. A new feature needs no
// deploy: once persisted it participates in resolution with its default.
func NewFeature(id uuid.UUID, key string, typ FeatureType, defaultValue any, description, limitBehavior, resetPeriod string, metadata map[string]any, now time.Time) (*Feature, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	f := &Feature{
		ID:            id,
		Key:           key,
		Type:          typ,
		DefaultValue:  defaultValue,
		Description:   description,
		LimitBehavior: limitBehavior,
		ResetPeriod:   resetPeriod,
		Metadata:      metadata,
		Active:        true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	return f, nil
}

// validate enforces the registry invariants.
func (f *Feature) validate() error {
	if f.Key == "" {
		return apperr.Validation("feature key is required")
	}
	if !f.Type.Valid() {
		return apperr.Validation("unknown feature type: " + string(f.Type))
	}
	switch f.LimitBehavior {
	case "", LimitSoft, LimitHard:
	default:
		return apperr.Validation("limit_behavior must be soft or hard")
	}
	if f.LimitBehavior != "" && f.Type != FeatureLimit {
		return apperr.Validation("limit_behavior only applies to limit features")
	}
	switch f.ResetPeriod {
	case "", ResetBillingCycle, ResetMonthly, ResetNever:
	default:
		return apperr.Validation("unknown reset_period: " + f.ResetPeriod)
	}
	// The default value must itself be a legal value for the type.
	if f.DefaultValue != nil {
		if err := f.ValidateValue(f.DefaultValue); err != nil {
			return err
		}
	}
	return nil
}

// AllowedValues returns an enum feature's allowed set, read from metadata.
func (f *Feature) AllowedValues() []string {
	raw, ok := f.Metadata[metadataAllowedValues]
	if !ok {
		return nil
	}
	switch vs := raw.(type) {
	case []string:
		return vs
	case []any:
		out := make([]string, 0, len(vs))
		for _, v := range vs {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// ValidateValue reports whether v is a legal value for this feature's type.
func (f *Feature) ValidateValue(v any) error {
	switch f.Type {
	case FeatureBoolean:
		if _, ok := v.(bool); !ok {
			return apperr.Validation("feature " + f.Key + " expects a boolean value")
		}
	case FeatureLimit:
		if _, ok := toInt64(v); !ok {
			return apperr.Validation("feature " + f.Key + " expects an integer limit value")
		}
	case FeatureConfig:
		// Config features accept any JSON value.
	case FeatureEnum:
		s, ok := v.(string)
		if !ok {
			return apperr.Validation("feature " + f.Key + " expects a string enum value")
		}
		allowed := f.AllowedValues()
		if len(allowed) > 0 && !contains(allowed, s) {
			return apperr.Validation("value " + s + " is not in the allowed set for feature " + f.Key)
		}
	}
	return nil
}

// Update applies mutable changes and re-validates. The key and type are
// immutable — a re-typed feature would corrupt existing entitlements.
func (f *Feature) Update(defaultValue any, description, limitBehavior, resetPeriod string, metadata map[string]any, now time.Time) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	f.DefaultValue = defaultValue
	f.Description = description
	f.LimitBehavior = limitBehavior
	f.ResetPeriod = resetPeriod
	f.Metadata = metadata
	f.UpdatedAt = now
	return f.validate()
}

// Archive marks the feature inactive. Archived features stop granting (they drop
// out of resolution) but their row and history are retained.
func (f *Feature) Archive(now time.Time) {
	f.Active = false
	f.UpdatedAt = now
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
