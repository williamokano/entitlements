package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/catalog/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// AddonView is a read model of an addon.
type AddonView struct {
	ID     uuid.UUID
	Key    string
	Name   string
	Status string
}

// AddonVersionView is a read model of an addon version.
type AddonVersionView struct {
	ID                 uuid.UUID
	AddonID            uuid.UUID
	Version            int
	Status             string
	Currency           string
	Prices             []domain.Price
	QuantityAllowed    bool
	CompatiblePlanKeys []string
	Deltas             []domain.Delta
}

// AddonVersionContent is the editable content of a draft addon version.
type AddonVersionContent struct {
	Currency           string
	Prices             []domain.Price
	QuantityAllowed    bool
	CompatiblePlanKeys []string
	Deltas             []domain.Delta
}

// CreateAddon creates a draft addon with an empty draft version v1.
func (s *Service) CreateAddon(ctx context.Context, key, name string) (AddonView, AddonVersionView, error) {
	now := s.clk.Now().UTC()
	addon, err := domain.NewAddon(s.ids.New(), key, name, now)
	if err != nil {
		return AddonView{}, AddonVersionView{}, err
	}
	version := domain.NewDraftAddonVersion(s.ids.New(), addon.ID, 1, now)
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.addons.CreateAddon(ctx, addon); err != nil {
			return err
		}
		return s.addonVersions.CreateVersion(ctx, version)
	}); err != nil {
		return AddonView{}, AddonVersionView{}, err
	}
	return toAddonView(addon), toAddonVersionView(version), nil
}

// UpdateDraftAddonVersion replaces a draft addon version's content.
func (s *Service) UpdateDraftAddonVersion(ctx context.Context, addonID, versionID uuid.UUID, content AddonVersionContent) (AddonVersionView, error) {
	var view AddonVersionView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		version, err := s.loadAddonVersion(ctx, addonID, versionID)
		if err != nil {
			return err
		}
		if err := version.SetContent(content.Currency, content.Prices, content.QuantityAllowed, content.CompatiblePlanKeys, content.Deltas); err != nil {
			return err
		}
		version.UpdatedAt = s.clk.Now().UTC()
		if err := s.addonVersions.UpdateVersion(ctx, version); err != nil {
			return err
		}
		view = toAddonVersionView(version)
		return nil
	})
	return view, err
}

// PublishAddonVersion freezes a draft addon version and activates the addon on
// its first publish.
func (s *Service) PublishAddonVersion(ctx context.Context, addonID, versionID uuid.UUID) (AddonVersionView, error) {
	var view AddonVersionView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		addon, err := s.addons.GetAddon(ctx, addonID)
		if err != nil {
			return err
		}
		version, err := s.loadAddonVersion(ctx, addonID, versionID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if err := version.Publish(now); err != nil {
			return err
		}
		if err := s.addonVersions.UpdateVersion(ctx, version); err != nil {
			return err
		}
		if addon.Status == domain.AddonDraft {
			if err := addon.Activate(); err != nil {
				return err
			}
			addon.UpdatedAt = now
			if err := s.addons.UpdateAddon(ctx, addon); err != nil {
				return err
			}
		}
		view = toAddonVersionView(version)
		return nil
	})
	return view, err
}

// CreateNewAddonVersion opens a new draft addon version seeded from the latest
// published one.
func (s *Service) CreateNewAddonVersion(ctx context.Context, addonID uuid.UUID) (AddonVersionView, error) {
	var view AddonVersionView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		if _, err := s.addons.GetAddon(ctx, addonID); err != nil {
			return err
		}
		if _, err := s.addonVersions.GetDraftVersion(ctx, addonID); err == nil {
			return apperr.Conflict("an unpublished draft addon version already exists")
		} else if apperr.KindOf(err) != apperr.KindNotFound {
			return err
		}
		maxVer, err := s.addonVersions.MaxVersionNumber(ctx, addonID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		draft := domain.NewDraftAddonVersion(s.ids.New(), addonID, maxVer+1, now)
		if latest, err := s.addonVersions.LatestPublishedVersion(ctx, addonID); err == nil {
			_ = draft.SetContent(latest.Currency, latest.Prices, latest.QuantityAllowed, latest.CompatiblePlanKeys, latest.Deltas)
		} else if apperr.KindOf(err) != apperr.KindNotFound {
			return err
		}
		if err := s.addonVersions.CreateVersion(ctx, draft); err != nil {
			return err
		}
		view = toAddonVersionView(draft)
		return nil
	})
	return view, err
}

// ArchiveAddon archives an addon.
func (s *Service) ArchiveAddon(ctx context.Context, addonID uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		addon, err := s.addons.GetAddon(ctx, addonID)
		if err != nil {
			return err
		}
		if err := addon.Archive(); err != nil {
			return err
		}
		addon.UpdatedAt = s.clk.Now().UTC()
		return s.addons.UpdateAddon(ctx, addon)
	})
}

// GetAddon returns an addon with its versions.
func (s *Service) GetAddon(ctx context.Context, addonID uuid.UUID) (AddonView, []AddonVersionView, error) {
	addon, err := s.addons.GetAddon(ctx, addonID)
	if err != nil {
		return AddonView{}, nil, err
	}
	versions, err := s.addonVersions.ListVersions(ctx, addonID)
	if err != nil {
		return AddonView{}, nil, err
	}
	views := make([]AddonVersionView, 0, len(versions))
	for _, v := range versions {
		views = append(views, toAddonVersionView(v))
	}
	return toAddonView(addon), views, nil
}

// ListAddons returns all addons.
func (s *Service) ListAddons(ctx context.Context) ([]AddonView, error) {
	addons, err := s.addons.ListAddons(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AddonView, 0, len(addons))
	for _, a := range addons {
		out = append(out, toAddonView(a))
	}
	return out, nil
}

// GetAddonVersion implements ports.CatalogReader — the frozen addon snapshot.
func (s *Service) GetAddonVersion(ctx context.Context, id uuid.UUID) (ports.AddonVersionInfo, error) {
	version, err := s.addonVersions.GetVersion(ctx, id)
	if err != nil {
		return ports.AddonVersionInfo{}, err
	}
	addon, err := s.addons.GetAddon(ctx, version.AddonID)
	if err != nil {
		return ports.AddonVersionInfo{}, err
	}
	prices := make([]ports.PriceInfo, 0, len(version.Prices))
	for _, p := range version.Prices {
		prices = append(prices, ports.PriceInfo{Cycle: string(p.Cycle), AmountMinor: p.AmountMinor})
	}
	deltas := make([]ports.DeltaInfo, 0, len(version.Deltas))
	for _, d := range version.Deltas {
		deltas = append(deltas, ports.DeltaInfo{FeatureKey: d.FeatureKey, Kind: string(d.Kind), Amount: d.Amount, Value: d.Value})
	}
	return ports.AddonVersionInfo{
		ID:                 version.ID,
		AddonID:            version.AddonID,
		AddonKey:           addon.Key,
		Version:            version.Version,
		Status:             string(version.Status),
		Currency:           version.Currency,
		Prices:             prices,
		QuantityAllowed:    version.QuantityAllowed,
		CompatiblePlanKeys: version.CompatiblePlanKeys,
		Deltas:             deltas,
	}, nil
}

func (s *Service) loadAddonVersion(ctx context.Context, addonID, versionID uuid.UUID) (*domain.AddonVersion, error) {
	version, err := s.addonVersions.GetVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version.AddonID != addonID {
		return nil, apperr.NotFound("addon version not found")
	}
	return version, nil
}

func toAddonView(a *domain.Addon) AddonView {
	return AddonView{ID: a.ID, Key: a.Key, Name: a.Name, Status: string(a.Status)}
}

func toAddonVersionView(v *domain.AddonVersion) AddonVersionView {
	prices := v.Prices
	if prices == nil {
		prices = []domain.Price{}
	}
	keys := v.CompatiblePlanKeys
	if keys == nil {
		keys = []string{}
	}
	deltas := v.Deltas
	if deltas == nil {
		deltas = []domain.Delta{}
	}
	return AddonVersionView{
		ID:                 v.ID,
		AddonID:            v.AddonID,
		Version:            v.Version,
		Status:             string(v.Status),
		Currency:           v.Currency,
		Prices:             prices,
		QuantityAllowed:    v.QuantityAllowed,
		CompatiblePlanKeys: keys,
		Deltas:             deltas,
	}
}
