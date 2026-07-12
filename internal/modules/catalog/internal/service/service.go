// Package service holds the catalog use cases: plan and version management. Each
// mutation and its published event share one transaction (via the outbox).
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/catalog/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Service implements the catalog use cases and ports.CatalogReader.
type Service struct {
	uow      *postgres.UnitOfWork
	outbox   *events.Outbox
	plans    domain.PlanRepository
	versions domain.PlanVersionRepository
	ids      id.Generator
	clk      clock.Clock
}

// New builds a Service.
func New(uow *postgres.UnitOfWork, outbox *events.Outbox, plans domain.PlanRepository, versions domain.PlanVersionRepository, ids id.Generator, clk clock.Clock) *Service {
	return &Service{uow: uow, outbox: outbox, plans: plans, versions: versions, ids: ids, clk: clk}
}

// VersionContent is the editable content of a draft version.
type VersionContent struct {
	Currency      string
	Prices        []domain.Price
	Trial         domain.TrialConfig
	GraceDays     int
	FeatureGrants map[string]any
}

// CreatePlan creates a draft plan with an empty draft version v1.
func (s *Service) CreatePlan(ctx context.Context, key, name string) (PlanView, VersionView, error) {
	now := s.clk.Now().UTC()
	plan, err := domain.NewPlan(s.ids.New(), key, name, now)
	if err != nil {
		return PlanView{}, VersionView{}, err
	}
	version := domain.NewDraftVersion(s.ids.New(), plan.ID, 1, now)
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.plans.CreatePlan(ctx, plan); err != nil {
			return err
		}
		return s.versions.CreateVersion(ctx, version)
	}); err != nil {
		return PlanView{}, VersionView{}, err
	}
	return toPlanView(plan), toVersionView(version), nil
}

// UpdateDraftVersion replaces a draft version's content. Published versions are
// immutable (the domain rejects the edit).
func (s *Service) UpdateDraftVersion(ctx context.Context, planID, versionID uuid.UUID, content VersionContent) (VersionView, error) {
	var view VersionView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		version, err := s.loadVersion(ctx, planID, versionID)
		if err != nil {
			return err
		}
		if err := version.SetContent(content.Currency, content.Prices, content.Trial, content.GraceDays, content.FeatureGrants); err != nil {
			return err
		}
		version.UpdatedAt = s.clk.Now().UTC()
		if err := s.versions.UpdateVersion(ctx, version); err != nil {
			return err
		}
		view = toVersionView(version)
		return nil
	})
	return view, err
}

// PublishVersion freezes a draft version and activates the plan on its first
// publish, then publishes catalog.plan_version.published.
func (s *Service) PublishVersion(ctx context.Context, planID, versionID uuid.UUID) (VersionView, error) {
	var view VersionView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		plan, err := s.plans.GetPlan(ctx, planID)
		if err != nil {
			return err
		}
		version, err := s.loadVersion(ctx, planID, versionID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if err := version.Publish(now); err != nil {
			return err
		}
		if err := s.versions.UpdateVersion(ctx, version); err != nil {
			return err
		}
		if plan.Status == domain.PlanDraft {
			if err := plan.Activate(); err != nil {
				return err
			}
			plan.UpdatedAt = now
			if err := s.plans.UpdatePlan(ctx, plan); err != nil {
				return err
			}
		}
		if _, err := s.outbox.Publish(ctx, events.EventInput{
			Module:  "catalog",
			Type:    ports.EventPlanVersionPublished,
			Payload: ports.PlanVersionPublished{PlanID: plan.ID, VersionID: version.ID, PlanKey: plan.Key, Version: version.Version},
		}); err != nil {
			return err
		}
		view = toVersionView(version)
		return nil
	})
	return view, err
}

// CreateNewVersion opens a new draft version seeded from the latest published
// version's content, so edits never mutate what existing subscribers pinned.
func (s *Service) CreateNewVersion(ctx context.Context, planID uuid.UUID) (VersionView, error) {
	var view VersionView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		if _, err := s.plans.GetPlan(ctx, planID); err != nil {
			return err
		}
		if _, err := s.versions.GetDraftVersion(ctx, planID); err == nil {
			return apperr.Conflict("an unpublished draft version already exists")
		} else if apperr.KindOf(err) != apperr.KindNotFound {
			return err
		}
		maxVer, err := s.versions.MaxVersionNumber(ctx, planID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		draft := domain.NewDraftVersion(s.ids.New(), planID, maxVer+1, now)
		// Seed from the latest published version, if any.
		if latest, err := s.versions.LatestPublishedVersion(ctx, planID); err == nil {
			_ = draft.SetContent(latest.Currency, latest.Prices, latest.Trial, latest.GraceDays, latest.FeatureGrants)
		} else if apperr.KindOf(err) != apperr.KindNotFound {
			return err
		}
		if err := s.versions.CreateVersion(ctx, draft); err != nil {
			return err
		}
		view = toVersionView(draft)
		return nil
	})
	return view, err
}

// SetPlanPublic toggles a plan's public visibility.
func (s *Service) SetPlanPublic(ctx context.Context, planID uuid.UUID, public bool) (PlanView, error) {
	var view PlanView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		plan, err := s.plans.GetPlan(ctx, planID)
		if err != nil {
			return err
		}
		plan.Public = public
		plan.UpdatedAt = s.clk.Now().UTC()
		if err := s.plans.UpdatePlan(ctx, plan); err != nil {
			return err
		}
		view = toPlanView(plan)
		return nil
	})
	return view, err
}

// ArchivePlan archives a plan and publishes catalog.plan.archived.
func (s *Service) ArchivePlan(ctx context.Context, planID uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		plan, err := s.plans.GetPlan(ctx, planID)
		if err != nil {
			return err
		}
		if err := plan.Archive(); err != nil {
			return err
		}
		plan.UpdatedAt = s.clk.Now().UTC()
		if err := s.plans.UpdatePlan(ctx, plan); err != nil {
			return err
		}
		_, err = s.outbox.Publish(ctx, events.EventInput{
			Module:  "catalog",
			Type:    ports.EventPlanArchived,
			Payload: ports.PlanArchived{PlanID: plan.ID, PlanKey: plan.Key},
		})
		return err
	})
}

// GetPlan returns a plan with its versions.
func (s *Service) GetPlan(ctx context.Context, planID uuid.UUID) (PlanView, []VersionView, error) {
	plan, err := s.plans.GetPlan(ctx, planID)
	if err != nil {
		return PlanView{}, nil, err
	}
	versions, err := s.versions.ListVersions(ctx, planID)
	if err != nil {
		return PlanView{}, nil, err
	}
	views := make([]VersionView, 0, len(versions))
	for _, v := range versions {
		views = append(views, toVersionView(v))
	}
	return toPlanView(plan), views, nil
}

// ListPlans returns all plans (admin view).
func (s *Service) ListPlans(ctx context.Context) ([]PlanView, error) {
	plans, err := s.plans.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	return toPlanViews(plans), nil
}

// PublicPlan is a public catalog entry: an active, public plan and its current
// published version.
type PublicPlan struct {
	Plan    PlanView
	Version VersionView
}

// ListPublicPlans returns active, public plans with their latest published
// version. Drafts and hidden plans are excluded.
func (s *Service) ListPublicPlans(ctx context.Context) ([]PublicPlan, error) {
	plans, err := s.plans.ListPublicPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublicPlan, 0, len(plans))
	for _, p := range plans {
		version, err := s.versions.LatestPublishedVersion(ctx, p.ID)
		if err != nil {
			if apperr.KindOf(err) == apperr.KindNotFound {
				continue // active+public but nothing published yet
			}
			return nil, err
		}
		out = append(out, PublicPlan{Plan: toPlanView(p), Version: toVersionView(version)})
	}
	return out, nil
}

// GetPlanVersion implements ports.CatalogReader — the frozen snapshot.
func (s *Service) GetPlanVersion(ctx context.Context, id uuid.UUID) (ports.PlanVersionInfo, error) {
	version, err := s.versions.GetVersion(ctx, id)
	if err != nil {
		return ports.PlanVersionInfo{}, err
	}
	plan, err := s.plans.GetPlan(ctx, version.PlanID)
	if err != nil {
		return ports.PlanVersionInfo{}, err
	}
	prices := make([]ports.PriceInfo, 0, len(version.Prices))
	for _, p := range version.Prices {
		prices = append(prices, ports.PriceInfo{Cycle: string(p.Cycle), AmountMinor: p.AmountMinor})
	}
	return ports.PlanVersionInfo{
		ID:            version.ID,
		PlanID:        version.PlanID,
		PlanKey:       plan.Key,
		Version:       version.Version,
		Status:        string(version.Status),
		Currency:      version.Currency,
		Prices:        prices,
		TrialEnabled:  version.Trial.Enabled,
		TrialDays:     version.Trial.Days,
		CardRequired:  version.Trial.CardRequired,
		GraceDays:     version.GraceDays,
		FeatureGrants: version.FeatureGrants,
	}, nil
}

// loadVersion loads a version and checks it belongs to the plan.
func (s *Service) loadVersion(ctx context.Context, planID, versionID uuid.UUID) (*domain.PlanVersion, error) {
	version, err := s.versions.GetVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version.PlanID != planID {
		return nil, apperr.NotFound("plan version not found")
	}
	return version, nil
}
