package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

// ConsumeHard atomically increments a hard-limit counter iff the increment stays
// within the limit, in a single guarded upsert. It returns the new usage and
// ok=true when the consume was accepted, or ok=false when it would breach the
// limit (no row was written). The guard lives in SQL, not Go, so N concurrent
// callers racing over a limit L can never push `used` past L: the initial insert
// is gated by `WHERE $n <= $limit` and every subsequent increment by
// `WHERE used + $n <= $limit`, and ON CONFLICT serializes concurrent writers on
// the row.
func (r *Repo) ConsumeHard(ctx context.Context, tenantID uuid.UUID, key, periodKey string, n, limit int64, now time.Time) (int64, bool, error) {
	var used int64
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`INSERT INTO entitlements.usage_counters
			(tenant_id, feature_key, period_key, used, warned, created_at, updated_at)
		 SELECT $1, $2, $3, $4::bigint, false, $5, $5
		 WHERE $4::bigint <= $6::bigint
		 ON CONFLICT (tenant_id, feature_key, period_key) DO UPDATE
			SET used = entitlements.usage_counters.used + $4::bigint, updated_at = $5
			WHERE entitlements.usage_counters.used + $4::bigint <= $6::bigint
		 RETURNING used`,
		tenantID, key, periodKey, n, now, limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("entitlements: consume hard: %w", err)
	}
	return used, true, nil
}

// Increment unconditionally adds n to a counter (creating it at n), returning the
// new usage. It backs soft limits and unlimited features, where consumption is
// never refused.
func (r *Repo) Increment(ctx context.Context, tenantID uuid.UUID, key, periodKey string, n int64, now time.Time) (int64, error) {
	var used int64
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`INSERT INTO entitlements.usage_counters
			(tenant_id, feature_key, period_key, used, warned, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, false, $5, $5)
		 ON CONFLICT (tenant_id, feature_key, period_key) DO UPDATE
			SET used = entitlements.usage_counters.used + $4, updated_at = $5
		 RETURNING used`,
		tenantID, key, periodKey, n, now).Scan(&used)
	if err != nil {
		return 0, fmt.Errorf("entitlements: increment usage: %w", err)
	}
	return used, nil
}

// ClaimWarning atomically flips the once-per-period `warned` latch from false to
// true, reporting whether this caller won the claim. Only the first concurrent
// caller past a soft-limit crossing gets claimed=true (the row lock serializes
// the guarded update), so exactly one EntitlementLimitWarning is emitted.
func (r *Repo) ClaimWarning(ctx context.Context, tenantID uuid.UUID, key, periodKey string, now time.Time) (bool, error) {
	var one int
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`UPDATE entitlements.usage_counters SET warned = true, updated_at = $4
		 WHERE tenant_id = $1 AND feature_key = $2 AND period_key = $3 AND warned = false
		 RETURNING 1`,
		tenantID, key, periodKey, now).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("entitlements: claim warning: %w", err)
	}
	return true, nil
}

// Release decrements a counter by n, floored at zero, returning the new usage. A
// missing counter (nothing consumed in the period) is a no-op that reports zero.
func (r *Repo) Release(ctx context.Context, tenantID uuid.UUID, key, periodKey string, n int64, now time.Time) (int64, error) {
	var used int64
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`UPDATE entitlements.usage_counters
			SET used = GREATEST(used - $4, 0), updated_at = $5
		 WHERE tenant_id = $1 AND feature_key = $2 AND period_key = $3
		 RETURNING used`,
		tenantID, key, periodKey, n, now).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("entitlements: release usage: %w", err)
	}
	return used, nil
}

// GetUsed returns the counter's current usage for a period, or zero when no row
// exists (a fresh or reset period).
func (r *Repo) GetUsed(ctx context.Context, tenantID uuid.UUID, key, periodKey string) (int64, error) {
	var used int64
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT used FROM entitlements.usage_counters
		 WHERE tenant_id = $1 AND feature_key = $2 AND period_key = $3`,
		tenantID, key, periodKey).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("entitlements: get usage: %w", err)
	}
	return used, nil
}
