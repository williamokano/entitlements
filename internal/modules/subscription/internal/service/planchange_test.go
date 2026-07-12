package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
)

// fakeRepo serves a single live subscription; validation-rejection paths never
// reach the write methods.
type fakeRepo struct {
	domain.Repository // panic on anything not overridden
	sub               *domain.Subscription
}

func (f *fakeRepo) GetLiveForTenant(_ context.Context, _ uuid.UUID) (*domain.Subscription, error) {
	return f.sub, nil
}

func (f *fakeRepo) GetAddon(_ context.Context, _, _ uuid.UUID) (*domain.Addon, error) {
	return nil, apperr.NotFound("addon not attached")
}

// fakeCatalog serves one plan version and one addon version.
type fakeCatalog struct {
	pv catalogports.PlanVersionInfo
	av catalogports.AddonVersionInfo
}

func (f fakeCatalog) GetPlanVersion(_ context.Context, _ uuid.UUID) (catalogports.PlanVersionInfo, error) {
	return f.pv, nil
}

func (f fakeCatalog) GetAddonVersion(_ context.Context, _ uuid.UUID) (catalogports.AddonVersionInfo, error) {
	return f.av, nil
}

func testService(t *testing.T, cat CatalogReader) (*Service, context.Context) {
	t.Helper()
	sub, err := domain.Create(uuid.New(), uuid.New(), uuid.New(), domain.CycleMonthly, 0,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "tester")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// uow and outbox are nil: the paths under test reject before any write.
	svc := New(nil, nil, &fakeRepo{sub: sub}, cat, id.UUIDv7{}, clock.NewFrozen(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), Config{})
	ctx := authctx.WithTenantID(context.Background(), sub.TenantID)
	return svc, ctx
}

func TestAddonAttachRejectsIncompatiblePlan(t *testing.T) {
	cat := fakeCatalog{
		pv: catalogports.PlanVersionInfo{PlanKey: "starter", Status: "published"},
		av: catalogports.AddonVersionInfo{Status: "published", QuantityAllowed: true, CompatiblePlanKeys: []string{"pro", "enterprise"}},
	}
	svc, ctx := testService(t, cat)
	_, err := svc.AttachAddon(ctx, uuid.New(), 1)
	if apperr.KindOf(err) != apperr.KindValidation {
		t.Fatalf("attach to incompatible plan = %v, want validation", err)
	}
}

func TestAddonAttachRejectsUnpublishedVersionAndBadQuantity(t *testing.T) {
	compatible := catalogports.AddonVersionInfo{Status: "published", QuantityAllowed: false, CompatiblePlanKeys: []string{"starter"}}
	pv := catalogports.PlanVersionInfo{PlanKey: "starter", Status: "published"}

	svc, ctx := testService(t, fakeCatalog{pv: pv, av: catalogports.AddonVersionInfo{Status: "draft", CompatiblePlanKeys: []string{"starter"}}})
	if _, err := svc.AttachAddon(ctx, uuid.New(), 1); apperr.KindOf(err) != apperr.KindValidation {
		t.Fatalf("attach draft version = %v, want validation", err)
	}

	svc, ctx = testService(t, fakeCatalog{pv: pv, av: compatible})
	if _, err := svc.AttachAddon(ctx, uuid.New(), 3); apperr.KindOf(err) != apperr.KindValidation {
		t.Fatalf("quantity 3 on non-quantity addon = %v, want validation", err)
	}
}

func TestIsUpgradeComparesMonthlyEquivalents(t *testing.T) {
	pvWith := func(cycle string, amount int64) catalogports.PlanVersionInfo {
		return catalogports.PlanVersionInfo{Prices: []catalogports.PriceInfo{{Cycle: cycle, AmountMinor: amount}}}
	}
	cases := []struct {
		name     string
		oldPV    catalogports.PlanVersionInfo
		oldCycle string
		newPV    catalogports.PlanVersionInfo
		newCycle string
		want     bool
	}{
		{"higher monthly is upgrade", pvWith("monthly", 1000), "monthly", pvWith("monthly", 2000), "monthly", true},
		{"lower monthly is downgrade", pvWith("monthly", 2000), "monthly", pvWith("monthly", 1000), "monthly", false},
		{"equal price counts as upgrade (immediate)", pvWith("monthly", 1000), "monthly", pvWith("monthly", 1000), "monthly", true},
		{"annual normalized: 24000/yr beats 1000/mo", pvWith("monthly", 1000), "monthly", pvWith("annual", 24000), "annual", true},
		{"annual normalized: 6000/yr under 1000/mo", pvWith("monthly", 1000), "monthly", pvWith("annual", 6000), "annual", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUpgrade(tc.oldPV, tc.oldCycle, tc.newPV, tc.newCycle); got != tc.want {
				t.Fatalf("isUpgrade = %v, want %v", got, tc.want)
			}
		})
	}
}
