package domain

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// VersionStatus is a plan version's state. A published version is immutable.
type VersionStatus string

// Version states.
const (
	VersionDraft     VersionStatus = "draft"
	VersionPublished VersionStatus = "published"
)

// BillingCycle is the interval a price is charged over.
type BillingCycle string

// Billing cycles.
const (
	CycleMonthly BillingCycle = "monthly"
	CycleAnnual  BillingCycle = "annual"
	CycleCustom  BillingCycle = "custom"
)

var validCycles = map[BillingCycle]bool{CycleMonthly: true, CycleAnnual: true, CycleCustom: true}

// Price is an amount charged per billing cycle, in integer minor units (never a
// float). The currency lives on the version so all of a version's prices share it.
type Price struct {
	Cycle       BillingCycle `json:"cycle"`
	AmountMinor int64        `json:"amount_minor"`
}

// TrialConfig configures a plan version's trial.
type TrialConfig struct {
	Enabled      bool `json:"enabled"`
	Days         int  `json:"days"`
	CardRequired bool `json:"card_required"`
}

// PlanVersion freezes pricing, trial/grace config, and feature grants. Once
// published it is immutable; edits create a new version instead.
type PlanVersion struct {
	ID            uuid.UUID
	PlanID        uuid.UUID
	Version       int
	Status        VersionStatus
	Currency      string
	Prices        []Price
	Trial         TrialConfig
	GraceDays     int
	FeatureGrants map[string]any
	PublishedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// NewDraftVersion builds a draft version number `version` for a plan.
func NewDraftVersion(id, planID uuid.UUID, version int, now time.Time) *PlanVersion {
	return &PlanVersion{
		ID:            id,
		PlanID:        planID,
		Version:       version,
		Status:        VersionDraft,
		Currency:      "",
		Prices:        []Price{},
		FeatureGrants: map[string]any{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// Published reports whether the version is frozen.
func (v *PlanVersion) Published() bool { return v.Status == VersionPublished }

// SetContent replaces the editable content of a draft version after validating
// it. Editing a published version is rejected — published versions are immutable.
func (v *PlanVersion) SetContent(currency string, prices []Price, trial TrialConfig, graceDays int, featureGrants map[string]any) error {
	if v.Published() {
		return apperr.Conflict("published plan version is immutable")
	}
	if err := validateContent(currency, prices, trial, graceDays); err != nil {
		return err
	}
	if featureGrants == nil {
		featureGrants = map[string]any{}
	}
	v.Currency = currency
	v.Prices = prices
	v.Trial = normalizeTrial(trial)
	v.GraceDays = graceDays
	v.FeatureGrants = featureGrants
	return nil
}

// Publish freezes the version at `now`. A version must have at least one price
// to be publishable, and cannot be published twice.
func (v *PlanVersion) Publish(now time.Time) error {
	if v.Published() {
		return apperr.Conflict("plan version is already published")
	}
	if err := validateContent(v.Currency, v.Prices, v.Trial, v.GraceDays); err != nil {
		return err
	}
	v.Status = VersionPublished
	v.PublishedAt = &now
	v.UpdatedAt = now
	return nil
}

func validateContent(currency string, prices []Price, trial TrialConfig, graceDays int) error {
	if !currencyPattern.MatchString(currency) {
		return apperr.Validation("currency must be a 3-letter ISO code (e.g. USD)")
	}
	if len(prices) == 0 {
		return apperr.Validation("a plan version needs at least one price")
	}
	seen := map[BillingCycle]bool{}
	for _, p := range prices {
		if !validCycles[p.Cycle] {
			return apperr.Validation(fmt.Sprintf("invalid billing cycle %q", p.Cycle))
		}
		if seen[p.Cycle] {
			return apperr.Validation(fmt.Sprintf("duplicate price for cycle %q", p.Cycle))
		}
		seen[p.Cycle] = true
		if p.AmountMinor < 0 {
			return apperr.Validation("price amount (minor units) must not be negative")
		}
	}
	if graceDays < 0 {
		return apperr.Validation("grace days must not be negative")
	}
	if trial.Enabled && trial.Days <= 0 {
		return apperr.Validation("trial days must be positive when the trial is enabled")
	}
	return nil
}

// normalizeTrial zeroes trial fields when the trial is disabled, so a disabled
// trial never carries stray days/card flags.
func normalizeTrial(t TrialConfig) TrialConfig {
	if !t.Enabled {
		return TrialConfig{Enabled: false, Days: 0, CardRequired: false}
	}
	return t
}

// PlanVersionRepository persists plan versions.
type PlanVersionRepository interface {
	CreateVersion(ctx context.Context, v *PlanVersion) error
	GetVersion(ctx context.Context, id uuid.UUID) (*PlanVersion, error)
	UpdateVersion(ctx context.Context, v *PlanVersion) error
	ListVersions(ctx context.Context, planID uuid.UUID) ([]*PlanVersion, error)
	GetDraftVersion(ctx context.Context, planID uuid.UUID) (*PlanVersion, error)
	LatestPublishedVersion(ctx context.Context, planID uuid.UUID) (*PlanVersion, error)
	MaxVersionNumber(ctx context.Context, planID uuid.UUID) (int, error)
}
