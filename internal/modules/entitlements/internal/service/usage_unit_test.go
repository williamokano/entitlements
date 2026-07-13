package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
)

// stubRepo implements the service Repository. Only the methods a given test
// exercises are wired; the rest are unreachable in that test and panic if hit.
type stubRepo struct {
	feature    *domain.Feature
	featureErr error
	incUsed    int64
}

func (s stubRepo) GetFeatureByKey(_ context.Context, _ string) (*domain.Feature, error) {
	return s.feature, s.featureErr
}

func (s stubRepo) Increment(_ context.Context, _ uuid.UUID, _, _ string, n int64, _ time.Time) (int64, error) {
	return s.incUsed + n, nil
}

// Unused Repository methods.
func (stubRepo) InsertFeature(context.Context, *domain.Feature) error          { panic("unused") }
func (stubRepo) UpdateFeature(context.Context, *domain.Feature) error          { panic("unused") }
func (stubRepo) ListFeatures(context.Context) ([]*domain.Feature, error)       { panic("unused") }
func (stubRepo) ListActiveFeatures(context.Context) ([]*domain.Feature, error) { panic("unused") }
func (stubRepo) ListLiveOverrides(context.Context, uuid.UUID, time.Time) (map[string]domain.LiveOverride, error) {
	panic("unused")
}
func (stubRepo) GetEffective(context.Context, uuid.UUID) (map[string]domain.Resolved, error) {
	panic("unused")
}
func (stubRepo) ReplaceEffective(context.Context, uuid.UUID, map[string]domain.Resolved, time.Time) error {
	panic("unused")
}
func (stubRepo) InsertOverride(context.Context, *domain.Override) error { panic("unused") }
func (stubRepo) UpdateOverride(context.Context, *domain.Override) error { panic("unused") }
func (stubRepo) GetOverride(context.Context, uuid.UUID, uuid.UUID) (*domain.Override, error) {
	panic("unused")
}
func (stubRepo) ListOverrides(context.Context, uuid.UUID) ([]*domain.Override, error) {
	panic("unused")
}
func (stubRepo) DeleteOverride(context.Context, uuid.UUID, uuid.UUID) error { panic("unused") }
func (stubRepo) ListExpiredOverrides(context.Context, time.Time) ([]*domain.Override, error) {
	panic("unused")
}
func (stubRepo) ConsumeHard(context.Context, uuid.UUID, string, string, int64, int64, time.Time) (int64, bool, error) {
	panic("unused")
}
func (stubRepo) ClaimWarning(context.Context, uuid.UUID, string, string, time.Time) (bool, error) {
	panic("unused")
}
func (stubRepo) Release(context.Context, uuid.UUID, string, string, int64, time.Time) (int64, error) {
	panic("unused")
}
func (stubRepo) GetUsed(context.Context, uuid.UUID, string, string) (int64, error) { panic("unused") }

func svcWith(repo Repository, policy domain.UnknownPolicy) *Service {
	return &Service{repo: repo, clk: clock.System, policy: policy}
}

func TestConsumeValidation(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()

	// n <= 0 is rejected before any resolution, regardless of feature/policy.
	limitFeature := &domain.Feature{Key: "api_calls", Type: domain.FeatureLimit, DefaultValue: int64(10)}
	svc := svcWith(stubRepo{feature: limitFeature}, domain.UnknownDeny)
	for _, n := range []int64{0, -1, -100} {
		if _, err := svc.ConsumeQuota(ctx, tenantID, "api_calls", n); apperr.KindOf(err) != apperr.KindValidation {
			t.Fatalf("ConsumeQuota n=%d err = %v, want validation", n, err)
		}
	}

	// Unknown feature under deny policy: the NotFound from the registry surfaces.
	denySvc := svcWith(stubRepo{featureErr: apperr.NotFound("feature not found")}, domain.UnknownDeny)
	if _, err := denySvc.ConsumeQuota(ctx, tenantID, "ghost", 1); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("unknown/deny err = %v, want not_found", err)
	}

	// Unknown feature under allow policy: consume succeeds as unlimited.
	allowSvc := svcWith(stubRepo{featureErr: apperr.NotFound("feature not found"), incUsed: 4}, domain.UnknownAllow)
	u, err := allowSvc.ConsumeQuota(ctx, tenantID, "ghost", 1)
	if err != nil {
		t.Fatalf("unknown/allow consume: %v", err)
	}
	if !u.Unlimited || u.Used != 5 {
		t.Fatalf("unknown/allow usage = %+v, want unlimited used=5", u)
	}

	// A non-limit feature is rejected as a validation error.
	boolSvc := svcWith(stubRepo{feature: &domain.Feature{Key: "sso", Type: domain.FeatureBoolean, DefaultValue: true}}, domain.UnknownDeny)
	if _, err := boolSvc.ConsumeQuota(ctx, tenantID, "sso", 1); apperr.KindOf(err) != apperr.KindValidation {
		t.Fatalf("non-limit feature err = %v, want validation", err)
	}
}
