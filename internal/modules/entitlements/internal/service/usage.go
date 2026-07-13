package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/entitlements/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// UsageView is the read model of a metered feature's usage, returned by the
// quota use cases. It aliases the port type so both the HTTP adapter and other
// modules see one shape.
type UsageView = ports.Usage

// quotaParams is everything a usage operation needs about a (tenant, feature):
// the effective limit for the current period, whether the feature is metered at
// all, the enforcement behavior, and the period key the counter lives under.
type quotaParams struct {
	limit     int64
	unlimited bool
	behavior  string
	periodKey string
}

// resolveQuota loads a feature, resolves its effective limit for the tenant, and
// computes the current period key. It enforces the unknown-feature policy (deny
// → NotFound; allow → an unlimited, untracked-but-counted feature) and rejects
// non-limit features. It is the shared prologue of every usage operation, so a
// consume always sees the same limit and period a read would.
func (s *Service) resolveQuota(ctx context.Context, tenantID uuid.UUID, key string) (quotaParams, error) {
	feat, err := s.repo.GetFeatureByKey(ctx, key)
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound && s.policy == domain.UnknownAllow {
			// Unknown feature under allow policy: no limit to enforce, but still
			// count usage under the fixed "never" period so reads report it.
			return quotaParams{unlimited: true, behavior: domain.LimitHard, periodKey: domain.PeriodKey(domain.ResetNever, s.clk.Now().UTC(), nil)}, nil
		}
		return quotaParams{}, err
	}
	if feat.Type != domain.FeatureLimit {
		return quotaParams{}, apperr.Validation("feature " + key + " is not a limit feature")
	}

	p := quotaParams{behavior: feat.EffectiveBehavior()}

	set, err := s.Resolve(ctx, tenantID)
	if err != nil {
		return quotaParams{}, err
	}
	value := feat.DefaultValue
	if r, ok := set[key]; ok {
		value = r.Value
	}
	if n, ok := domain.ToInt64(value); ok {
		p.limit = n
	} else {
		p.unlimited = true
	}

	p.periodKey = domain.PeriodKey(feat.ResetPeriod, s.clk.Now().UTC(), s.periodStart(ctx, tenantID, feat.ResetPeriod))
	return p, nil
}

// periodStart returns the subscription's current-period start when the feature
// resets per billing cycle, and nil otherwise (monthly/never anchor on the clock
// alone, and a tenant without a live subscription has no cycle to anchor to).
func (s *Service) periodStart(ctx context.Context, tenantID uuid.UUID, resetPeriod string) *time.Time {
	if resetPeriod != domain.ResetBillingCycle {
		return nil
	}
	sub, err := s.subs.GetLiveForTenant(ctx, tenantID)
	if err != nil {
		return nil
	}
	start := sub.CurrentPeriodStart
	return &start
}

// ConsumeQuota atomically consumes n units of a metered feature for a tenant.
//
// Hard limits are enforced in a single guarded SQL statement, so N concurrent
// callers racing over a limit L result in exactly L accepted consumes and the
// rest returning a KindQuotaExceeded error — nothing is ever over-consumed.
//
// Soft limits always consume; the consume that crosses the limit
// (before ≤ limit < after) emits exactly one EntitlementLimitWarning, claimed
// via a per-period latch so concurrent crossers emit a single event.
//
// n must be positive; unknown features honor the unknown-feature policy.
func (s *Service) ConsumeQuota(ctx context.Context, tenantID uuid.UUID, key string, n int64) (ports.Usage, error) {
	if n <= 0 {
		return ports.Usage{}, apperr.Validation("consume amount must be positive")
	}
	p, err := s.resolveQuota(ctx, tenantID, key)
	if err != nil {
		return ports.Usage{}, err
	}
	now := s.clk.Now().UTC()

	// Unlimited: always accept, still count.
	if p.unlimited {
		used, err := s.repo.Increment(ctx, tenantID, key, p.periodKey, n, now)
		if err != nil {
			return ports.Usage{}, err
		}
		return s.usageView(key, used, p), nil
	}

	if p.behavior == domain.LimitHard {
		used, ok, err := s.repo.ConsumeHard(ctx, tenantID, key, p.periodKey, n, p.limit, now)
		if err != nil {
			return ports.Usage{}, err
		}
		if !ok {
			return ports.Usage{}, apperr.QuotaExceeded("quota exceeded for feature " + key)
		}
		return s.usageView(key, used, p), nil
	}

	// Soft: consume unconditionally, then emit one warning on the crossing. The
	// increment, latch claim, and event publish share one transaction so the
	// warning commits atomically with the crossing consume.
	var used int64
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		after, err := s.repo.Increment(ctx, tenantID, key, p.periodKey, n, now)
		if err != nil {
			return err
		}
		used = after
		before := after - n
		if before <= p.limit && after > p.limit {
			claimed, err := s.repo.ClaimWarning(ctx, tenantID, key, p.periodKey, now)
			if err != nil {
				return err
			}
			if claimed {
				if _, err := s.outbox.Publish(ctx, events.EventInput{
					TenantID: tenantID,
					Module:   "entitlements",
					Type:     ports.EventEntitlementLimitWarning,
					Payload:  ports.UsageEvent{TenantID: tenantID, FeatureKey: key, Used: after, Limit: p.limit, Period: p.periodKey},
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return ports.Usage{}, err
	}
	return s.usageView(key, used, p), nil
}

// ReleaseQuota returns n units of a metered feature to the tenant's current
// period, floored at zero. n must be positive.
func (s *Service) ReleaseQuota(ctx context.Context, tenantID uuid.UUID, key string, n int64) (ports.Usage, error) {
	if n <= 0 {
		return ports.Usage{}, apperr.Validation("release amount must be positive")
	}
	p, err := s.resolveQuota(ctx, tenantID, key)
	if err != nil {
		return ports.Usage{}, err
	}
	used, err := s.repo.Release(ctx, tenantID, key, p.periodKey, n, s.clk.Now().UTC())
	if err != nil {
		return ports.Usage{}, err
	}
	return s.usageView(key, used, p), nil
}

// GetUsage reports one metered feature's current usage against its effective
// limit for the tenant's current period.
func (s *Service) GetUsage(ctx context.Context, tenantID uuid.UUID, key string) (ports.Usage, error) {
	p, err := s.resolveQuota(ctx, tenantID, key)
	if err != nil {
		return ports.Usage{}, err
	}
	used, err := s.repo.GetUsed(ctx, tenantID, key, p.periodKey)
	if err != nil {
		return ports.Usage{}, err
	}
	return s.usageView(key, used, p), nil
}

// ListUsage reports current usage for every active limit feature.
func (s *Service) ListUsage(ctx context.Context, tenantID uuid.UUID) ([]ports.Usage, error) {
	features, err := s.repo.ListActiveFeatures(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ports.Usage, 0)
	for _, f := range features {
		if f.Type != domain.FeatureLimit {
			continue
		}
		u, err := s.GetUsage(ctx, tenantID, f.Key)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *Service) usageView(key string, used int64, p quotaParams) ports.Usage {
	return ports.Usage{
		Key:       key,
		Used:      used,
		Limit:     p.limit,
		Unlimited: p.unlimited,
		Period:    p.periodKey,
		Behavior:  p.behavior,
	}
}

// emitDowngradeExceeded runs inside Materialize's transaction, after the
// effective set is known to have changed. For every limit feature whose new
// effective limit dropped below the tenant's current usage, it emits one
// EntitlementExceeded event. It never blocks reads — only future consumes are
// gated by the new limit — and, because Materialize only reaches here on a real
// change (and its consumers are idempotent), a given downgrade emits once.
func (s *Service) emitDowngradeExceeded(ctx context.Context, tenantID uuid.UUID, stored, resolved map[string]domain.Resolved) error {
	features, err := s.repo.ListActiveFeatures(ctx)
	if err != nil {
		return err
	}
	now := s.clk.Now().UTC()
	for _, f := range features {
		if f.Type != domain.FeatureLimit {
			continue
		}
		r, ok := resolved[f.Key]
		if !ok {
			continue
		}
		newLimit, ok := domain.ToInt64(r.Value)
		if !ok {
			continue // unlimited: nothing to breach
		}
		periodKey := domain.PeriodKey(f.ResetPeriod, now, s.periodStart(ctx, tenantID, f.ResetPeriod))
		used, err := s.repo.GetUsed(ctx, tenantID, f.Key, periodKey)
		if err != nil {
			return err
		}
		if used <= newLimit {
			continue
		}
		// Emit only on an actual shrink: the limit fell (or the feature newly
		// appeared) and now sits below usage.
		if prev, had := stored[f.Key]; had {
			if oldLimit, ok := domain.ToInt64(prev.Value); ok && oldLimit <= newLimit {
				continue
			}
		}
		if _, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenantID,
			Module:   "entitlements",
			Type:     ports.EventEntitlementExceeded,
			Payload:  ports.UsageEvent{TenantID: tenantID, FeatureKey: f.Key, Used: used, Limit: newLimit, Period: periodKey},
		}); err != nil {
			return err
		}
	}
	return nil
}
