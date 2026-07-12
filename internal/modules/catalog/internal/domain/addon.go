package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// AddonStatus is an addon's lifecycle state (same shape as a plan's).
type AddonStatus string

// Addon lifecycle states.
const (
	AddonDraft    AddonStatus = "draft"
	AddonActive   AddonStatus = "active"
	AddonArchived AddonStatus = "archived"
)

var allowedAddonTransitions = map[AddonStatus][]AddonStatus{
	AddonDraft:    {AddonActive, AddonArchived},
	AddonActive:   {AddonArchived},
	AddonArchived: {},
}

// Addon extends a plan with extra pricing and entitlement deltas. Its content
// lives in versions; a published version is immutable.
type Addon struct {
	ID        uuid.UUID
	Key       string
	Name      string
	Status    AddonStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewAddon builds a draft addon with a validated key.
func NewAddon(id uuid.UUID, key, name string, now time.Time) (*Addon, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	if !keyPattern.MatchString(k) {
		return nil, apperr.Validation("addon key must be lowercase alphanumerics and single hyphens")
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Validation("addon name must not be empty")
	}
	return &Addon{ID: id, Key: k, Name: name, Status: AddonDraft, CreatedAt: now, UpdatedAt: now}, nil
}

func (a *Addon) transitionTo(to AddonStatus) error {
	for _, s := range allowedAddonTransitions[a.Status] {
		if s == to {
			a.Status = to
			return nil
		}
	}
	return apperr.Conflict(fmt.Sprintf("cannot transition addon from %s to %s", a.Status, to))
}

// Activate moves a draft addon to active (on its first version publish).
func (a *Addon) Activate() error { return a.transitionTo(AddonActive) }

// Archive moves an addon to archived.
func (a *Addon) Archive() error { return a.transitionTo(AddonArchived) }

// DeltaKind is the kind of entitlement change an addon applies to a feature.
type DeltaKind string

// Delta kinds.
const (
	// DeltaLimit adds Amount to a numeric (limit) feature.
	DeltaLimit DeltaKind = "limit_delta"
	// DeltaOverride sets a feature to Value.
	DeltaOverride DeltaKind = "value_override"
)

// Delta is an entitlement change an addon applies to a feature key.
type Delta struct {
	FeatureKey string    `json:"feature_key"`
	Kind       DeltaKind `json:"kind"`
	Amount     int64     `json:"amount,omitempty"`
	Value      any       `json:"value,omitempty"`
}

// AddonVersion freezes an addon's pricing, compatibility, and entitlement deltas.
type AddonVersion struct {
	ID                 uuid.UUID
	AddonID            uuid.UUID
	Version            int
	Status             VersionStatus
	Currency           string
	Prices             []Price
	QuantityAllowed    bool
	CompatiblePlanKeys []string
	Deltas             []Delta
	PublishedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// NewDraftAddonVersion builds a draft addon version.
func NewDraftAddonVersion(id, addonID uuid.UUID, version int, now time.Time) *AddonVersion {
	return &AddonVersion{
		ID:                 id,
		AddonID:            addonID,
		Version:            version,
		Status:             VersionDraft,
		Prices:             []Price{},
		CompatiblePlanKeys: []string{},
		Deltas:             []Delta{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// Published reports whether the addon version is frozen.
func (v *AddonVersion) Published() bool { return v.Status == VersionPublished }

// SetContent replaces a draft addon version's content after validating it.
// Editing a published version is rejected.
func (v *AddonVersion) SetContent(currency string, prices []Price, quantityAllowed bool, compatiblePlanKeys []string, deltas []Delta) error {
	if v.Published() {
		return apperr.Conflict("published addon version is immutable")
	}
	if err := ValidateCurrencyAndPrices(currency, prices); err != nil {
		return err
	}
	if err := validateDeltas(deltas); err != nil {
		return err
	}
	if compatiblePlanKeys == nil {
		compatiblePlanKeys = []string{}
	}
	if deltas == nil {
		deltas = []Delta{}
	}
	v.Currency = currency
	v.Prices = prices
	v.QuantityAllowed = quantityAllowed
	v.CompatiblePlanKeys = compatiblePlanKeys
	v.Deltas = deltas
	return nil
}

// Publish freezes the addon version at now.
func (v *AddonVersion) Publish(now time.Time) error {
	if v.Published() {
		return apperr.Conflict("addon version is already published")
	}
	if err := ValidateCurrencyAndPrices(v.Currency, v.Prices); err != nil {
		return err
	}
	if err := validateDeltas(v.Deltas); err != nil {
		return err
	}
	v.Status = VersionPublished
	v.PublishedAt = &now
	v.UpdatedAt = now
	return nil
}

// CompatibleWith reports whether the addon may attach to a plan of planKey.
func (v *AddonVersion) CompatibleWith(planKey string) bool {
	for _, k := range v.CompatiblePlanKeys {
		if k == planKey {
			return true
		}
	}
	return false
}

// EnsureCompatible returns a typed error when the addon is not compatible with
// planKey. This is the shared compatibility check other modules reuse.
func (v *AddonVersion) EnsureCompatible(planKey string) error {
	if !v.CompatibleWith(planKey) {
		return apperr.Validation(fmt.Sprintf("addon is not compatible with plan %q", planKey))
	}
	return nil
}

// ValidateQuantity checks a requested attach quantity: it must be at least 1,
// and more than 1 only when the addon allows quantities.
func (v *AddonVersion) ValidateQuantity(qty int) error {
	if qty < 1 {
		return apperr.Validation("quantity must be at least 1")
	}
	if qty > 1 && !v.QuantityAllowed {
		return apperr.Validation("this addon does not allow quantity greater than 1")
	}
	return nil
}

func validateDeltas(deltas []Delta) error {
	seen := map[string]bool{}
	for _, d := range deltas {
		if strings.TrimSpace(d.FeatureKey) == "" {
			return apperr.Validation("delta feature_key must not be empty")
		}
		if seen[d.FeatureKey] {
			return apperr.Validation(fmt.Sprintf("duplicate delta for feature %q", d.FeatureKey))
		}
		seen[d.FeatureKey] = true
		switch d.Kind {
		case DeltaLimit:
			if d.Value != nil {
				return apperr.Validation("limit_delta must not carry a value")
			}
		case DeltaOverride:
			if d.Value == nil {
				return apperr.Validation("value_override requires a value")
			}
			if d.Amount != 0 {
				return apperr.Validation("value_override must not carry an amount")
			}
		default:
			return apperr.Validation(fmt.Sprintf("invalid delta kind %q", d.Kind))
		}
	}
	return nil
}

// AddonRepository persists addons.
type AddonRepository interface {
	CreateAddon(ctx context.Context, a *Addon) error
	GetAddon(ctx context.Context, id uuid.UUID) (*Addon, error)
	UpdateAddon(ctx context.Context, a *Addon) error
	ListAddons(ctx context.Context) ([]*Addon, error)
}

// AddonVersionRepository persists addon versions.
type AddonVersionRepository interface {
	CreateVersion(ctx context.Context, v *AddonVersion) error
	GetVersion(ctx context.Context, id uuid.UUID) (*AddonVersion, error)
	UpdateVersion(ctx context.Context, v *AddonVersion) error
	ListVersions(ctx context.Context, addonID uuid.UUID) ([]*AddonVersion, error)
	GetDraftVersion(ctx context.Context, addonID uuid.UUID) (*AddonVersion, error)
	LatestPublishedVersion(ctx context.Context, addonID uuid.UUID) (*AddonVersion, error)
	MaxVersionNumber(ctx context.Context, addonID uuid.UUID) (int, error)
}
