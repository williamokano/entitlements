// Package service holds the entitlements use cases: the feature-registry CRUD,
// the resolution pipeline (plan grants → addon deltas → tenant overrides), and
// the materialization that rebuilds a tenant's effective set and publishes an
// EntitlementsSummaryChanged event when it changes.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/entitlements/ports"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Repository is the persistence the service needs.
type Repository interface {
	InsertFeature(ctx context.Context, f *domain.Feature) error
	UpdateFeature(ctx context.Context, f *domain.Feature) error
	GetFeatureByKey(ctx context.Context, key string) (*domain.Feature, error)
	ListFeatures(ctx context.Context) ([]*domain.Feature, error)
	ListActiveFeatures(ctx context.Context) ([]*domain.Feature, error)
	ListLiveOverrides(ctx context.Context, tenantID uuid.UUID, now time.Time) (map[string]any, error)
	GetEffective(ctx context.Context, tenantID uuid.UUID) (map[string]domain.Resolved, error)
	ReplaceEffective(ctx context.Context, tenantID uuid.UUID, set map[string]domain.Resolved, now time.Time) error
}

// CatalogReader is the slice of the catalog the resolver reads: the pinned plan
// version's feature grants and each attached addon version's deltas.
type CatalogReader interface {
	GetPlanVersion(ctx context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error)
	GetAddonVersion(ctx context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error)
}

// SubscriptionReader is the slice of the subscription module the resolver reads:
// the tenant's live subscription (its pinned plan version) and its attached
// addons with quantities.
type SubscriptionReader interface {
	GetLiveForTenant(ctx context.Context, tenantID uuid.UUID) (subports.SubscriptionInfo, error)
	GetAttachedAddons(ctx context.Context, subscriptionID uuid.UUID) ([]subports.AddonAttachment, error)
}

// Service implements the entitlements use cases and ports.EntitlementsReader.
type Service struct {
	uow     *postgres.UnitOfWork
	outbox  *events.Outbox
	repo    Repository
	catalog CatalogReader
	subs    SubscriptionReader
	ids     id.Generator
	clk     clock.Clock
	policy  domain.UnknownPolicy
}

// Config carries the entitlements policy knobs.
type Config struct {
	UnknownFeaturePolicy domain.UnknownPolicy
}

// New builds a Service.
func New(uow *postgres.UnitOfWork, outbox *events.Outbox, repo Repository, catalog CatalogReader, subs SubscriptionReader, ids id.Generator, clk clock.Clock, cfg Config) *Service {
	policy := cfg.UnknownFeaturePolicy
	if policy != domain.UnknownAllow {
		policy = domain.UnknownDeny
	}
	return &Service{uow: uow, outbox: outbox, repo: repo, catalog: catalog, subs: subs, ids: ids, clk: clk, policy: policy}
}

// FeatureView is the read model of a registry feature.
type FeatureView struct {
	Key           string
	Type          string
	DefaultValue  any
	Description   string
	LimitBehavior string
	ResetPeriod   string
	Metadata      map[string]any
	Active        bool
}

// FeatureInput is the create/update payload for a feature.
type FeatureInput struct {
	Key           string
	Type          string
	DefaultValue  any
	Description   string
	LimitBehavior string
	ResetPeriod   string
	Metadata      map[string]any
}

// CreateFeature registers a new feature. It participates in resolution
// immediately — no deploy required.
func (s *Service) CreateFeature(ctx context.Context, in FeatureInput) (FeatureView, error) {
	now := s.clk.Now().UTC()
	f, err := domain.NewFeature(s.ids.New(), in.Key, domain.FeatureType(in.Type), in.DefaultValue, in.Description, in.LimitBehavior, in.ResetPeriod, in.Metadata, now)
	if err != nil {
		return FeatureView{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error { return s.repo.InsertFeature(ctx, f) }); err != nil {
		return FeatureView{}, err
	}
	return toFeatureView(f), nil
}

// UpdateFeature mutates a feature's editable fields (key and type are immutable).
func (s *Service) UpdateFeature(ctx context.Context, key string, in FeatureInput) (FeatureView, error) {
	f, err := s.repo.GetFeatureByKey(ctx, key)
	if err != nil {
		return FeatureView{}, err
	}
	now := s.clk.Now().UTC()
	if err := f.Update(in.DefaultValue, in.Description, in.LimitBehavior, in.ResetPeriod, in.Metadata, now); err != nil {
		return FeatureView{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error { return s.repo.UpdateFeature(ctx, f) }); err != nil {
		return FeatureView{}, err
	}
	return toFeatureView(f), nil
}

// ArchiveFeature marks a feature inactive: it stops granting but its row and
// history are retained.
func (s *Service) ArchiveFeature(ctx context.Context, key string) (FeatureView, error) {
	f, err := s.repo.GetFeatureByKey(ctx, key)
	if err != nil {
		return FeatureView{}, err
	}
	f.Archive(s.clk.Now().UTC())
	if err := s.uow.Do(ctx, func(ctx context.Context) error { return s.repo.UpdateFeature(ctx, f) }); err != nil {
		return FeatureView{}, err
	}
	return toFeatureView(f), nil
}

// GetFeature returns one registry feature.
func (s *Service) GetFeature(ctx context.Context, key string) (FeatureView, error) {
	f, err := s.repo.GetFeatureByKey(ctx, key)
	if err != nil {
		return FeatureView{}, err
	}
	return toFeatureView(f), nil
}

// ListFeatures returns every registry feature (active and archived).
func (s *Service) ListFeatures(ctx context.Context) ([]FeatureView, error) {
	fs, err := s.repo.ListFeatures(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FeatureView, 0, len(fs))
	for _, f := range fs {
		out = append(out, toFeatureView(f))
	}
	return out, nil
}

// Resolve composes a tenant's effective entitlement set live from the registry,
// its live subscription's pinned plan version + attached addons, and its
// overrides. It is the single source of truth reads and materialization share.
func (s *Service) Resolve(ctx context.Context, tenantID uuid.UUID) (map[string]domain.Resolved, error) {
	features, err := s.repo.ListActiveFeatures(ctx)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]domain.Feature, len(features))
	for _, f := range features {
		registry[f.Key] = *f
	}

	in := domain.ResolveInput{Features: registry, Policy: s.policy}

	sub, err := s.subs.GetLiveForTenant(ctx, tenantID)
	switch {
	case err == nil:
		pv, err := s.catalog.GetPlanVersion(ctx, sub.PlanVersionID)
		if err != nil {
			return nil, err
		}
		in.PlanGrants = pv.FeatureGrants

		addons, err := s.subs.GetAttachedAddons(ctx, sub.ID)
		if err != nil {
			return nil, err
		}
		for _, a := range addons {
			av, err := s.catalog.GetAddonVersion(ctx, a.AddonVersionID)
			if err != nil {
				return nil, err
			}
			for _, d := range av.Deltas {
				in.AddonDeltas = append(in.AddonDeltas, domain.AppliedDelta{
					FeatureKey: d.FeatureKey,
					Kind:       d.Kind,
					Amount:     d.Amount,
					Value:      d.Value,
					Quantity:   a.Quantity,
				})
			}
		}
	case apperr.KindOf(err) == apperr.KindNotFound:
		// No live subscription: the tenant gets registry defaults plus overrides.
	default:
		return nil, err
	}

	overrides, err := s.repo.ListLiveOverrides(ctx, tenantID, s.clk.Now().UTC())
	if err != nil {
		return nil, err
	}
	in.Overrides = overrides

	return domain.Resolve(in)
}

// Materialize re-resolves a tenant's effective set, diffs it against the stored
// set, and — only on a change — rewrites the materialized rows and publishes
// exactly one EntitlementsSummaryChanged carrying the full set. The rebuild and
// publish share one transaction, so they commit together. A no-op change
// publishes nothing.
func (s *Service) Materialize(ctx context.Context, tenantID uuid.UUID) error {
	resolved, err := s.Resolve(ctx, tenantID)
	if err != nil {
		return err
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		stored, err := s.repo.GetEffective(ctx, tenantID)
		if err != nil {
			return err
		}
		if sameSet(stored, resolved) {
			return nil // no change: no rewrite, no event
		}
		now := s.clk.Now().UTC()
		if err := s.repo.ReplaceEffective(ctx, tenantID, resolved, now); err != nil {
			return err
		}
		entitlements := make(map[string]ports.SummaryEntry, len(resolved))
		for key, r := range resolved {
			entitlements[key] = ports.SummaryEntry{Value: r.Value, Source: r.Source}
		}
		_, err = s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenantID,
			Module:   "entitlements",
			Type:     ports.EventEntitlementsSummaryChanged,
			Payload:  ports.EntitlementsSummaryChanged{TenantID: tenantID, Entitlements: entitlements},
		})
		return err
	})
}

// Get implements ports.EntitlementsReader: one feature's effective entitlement.
func (s *Service) Get(ctx context.Context, tenantID uuid.UUID, key string) (ports.Entitlement, error) {
	set, err := s.Resolve(ctx, tenantID)
	if err != nil {
		return ports.Entitlement{}, err
	}
	r, ok := set[key]
	if !ok {
		return ports.Entitlement{}, apperr.NotFound("no entitlement for feature " + key)
	}
	return ports.Entitlement{Key: key, Value: r.Value, Source: r.Source}, nil
}

// GetAll implements ports.EntitlementsReader: the tenant's whole effective set.
func (s *Service) GetAll(ctx context.Context, tenantID uuid.UUID) (map[string]ports.Entitlement, error) {
	set, err := s.Resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.Entitlement, len(set))
	for key, r := range set {
		out[key] = ports.Entitlement{Key: key, Value: r.Value, Source: r.Source}
	}
	return out, nil
}

// sameSet reports whether two effective sets are identical (same keys, sources,
// and JSON-equal values), so materialization skips a no-op change.
func sameSet(a map[string]domain.Resolved, b map[string]domain.Resolved) bool {
	if len(a) != len(b) {
		return false
	}
	for key, av := range a {
		bv, ok := b[key]
		if !ok || av.Source != bv.Source {
			return false
		}
		aj, err1 := json.Marshal(av.Value)
		bj, err2 := json.Marshal(bv.Value)
		if err1 != nil || err2 != nil || string(aj) != string(bj) {
			return false
		}
	}
	return true
}

func toFeatureView(f *domain.Feature) FeatureView {
	meta := f.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	return FeatureView{
		Key:           f.Key,
		Type:          string(f.Type),
		DefaultValue:  f.DefaultValue,
		Description:   f.Description,
		LimitBehavior: f.LimitBehavior,
		ResetPeriod:   f.ResetPeriod,
		Metadata:      meta,
		Active:        f.Active,
	}
}
