// Package postgres is the entitlements module's driven adapter: the feature
// registry, tenant-override reads, and the materialized effective-entitlements
// store, all over the platform pgx pool and routed through postgres.Q so writes
// join the ambient UnitOfWork transaction.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

const uniqueViolation = "23505"

// Repo is a Postgres-backed store for features, overrides, and the effective set.
type Repo struct {
	pool *pgxpool.Pool
}

// New builds a Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// InsertFeature persists a new feature, mapping a duplicate key to a conflict.
func (r *Repo) InsertFeature(ctx context.Context, f *domain.Feature) error {
	def, err := json.Marshal(f.DefaultValue)
	if err != nil {
		return fmt.Errorf("entitlements: marshal default: %w", err)
	}
	meta, err := json.Marshal(f.Metadata)
	if err != nil {
		return fmt.Errorf("entitlements: marshal metadata: %w", err)
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO entitlements.features
			(id, key, type, default_value, description, limit_behavior, reset_period, metadata, active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8::jsonb, $9, $10, $11)`,
		f.ID, f.Key, string(f.Type), string(def), f.Description, f.LimitBehavior, f.ResetPeriod, string(meta), f.Active, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("feature key already exists")
		}
		return fmt.Errorf("entitlements: insert feature: %w", err)
	}
	return nil
}

// UpdateFeature persists a feature's mutable fields (key and type are immutable).
func (r *Repo) UpdateFeature(ctx context.Context, f *domain.Feature) error {
	def, err := json.Marshal(f.DefaultValue)
	if err != nil {
		return fmt.Errorf("entitlements: marshal default: %w", err)
	}
	meta, err := json.Marshal(f.Metadata)
	if err != nil {
		return fmt.Errorf("entitlements: marshal metadata: %w", err)
	}
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE entitlements.features
			SET default_value = $2::jsonb, description = $3, limit_behavior = $4,
			    reset_period = $5, metadata = $6::jsonb, active = $7, updated_at = $8
		 WHERE id = $1`,
		f.ID, string(def), f.Description, f.LimitBehavior, f.ResetPeriod, string(meta), f.Active, f.UpdatedAt)
	if err != nil {
		return fmt.Errorf("entitlements: update feature: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("feature not found")
	}
	return nil
}

// GetFeatureByKey returns one feature by its key.
func (r *Repo) GetFeatureByKey(ctx context.Context, key string) (*domain.Feature, error) {
	row := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT `+featureColumns+` FROM entitlements.features WHERE key = $1`, key)
	f, err := scanFeature(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("feature not found")
	}
	if err != nil {
		return nil, fmt.Errorf("entitlements: get feature: %w", err)
	}
	return f, nil
}

// ListFeatures returns every feature (active and archived), newest key order.
func (r *Repo) ListFeatures(ctx context.Context) ([]*domain.Feature, error) {
	return r.queryFeatures(ctx, `ORDER BY key`)
}

// ListActiveFeatures returns only active features (the resolution registry).
func (r *Repo) ListActiveFeatures(ctx context.Context) ([]*domain.Feature, error) {
	return r.queryFeatures(ctx, `WHERE active = true ORDER BY key`)
}

func (r *Repo) queryFeatures(ctx context.Context, clause string) ([]*domain.Feature, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT `+featureColumns+` FROM entitlements.features `+clause)
	if err != nil {
		return nil, fmt.Errorf("entitlements: list features: %w", err)
	}
	defer rows.Close()

	var out []*domain.Feature
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			return nil, fmt.Errorf("entitlements: scan feature: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entitlements: iterate features: %w", err)
	}
	return out, nil
}

const featureColumns = `id, key, type, default_value, description, limit_behavior, reset_period, metadata, active, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFeature(row rowScanner) (*domain.Feature, error) {
	var (
		f       domain.Feature
		typ     string
		defJSON []byte
		metaRaw []byte
	)
	if err := row.Scan(&f.ID, &f.Key, &typ, &defJSON, &f.Description, &f.LimitBehavior, &f.ResetPeriod, &metaRaw, &f.Active, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	f.Type = domain.FeatureType(typ)
	if len(defJSON) > 0 {
		if err := json.Unmarshal(defJSON, &f.DefaultValue); err != nil {
			return nil, fmt.Errorf("entitlements: unmarshal default: %w", err)
		}
	}
	f.Metadata = map[string]any{}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &f.Metadata); err != nil {
			return nil, fmt.Errorf("entitlements: unmarshal metadata: %w", err)
		}
	}
	return &f, nil
}

// ListLiveOverrides returns a tenant's non-expired overrides, keyed by feature,
// each paired with its expiry (nil when it never expires). When two rows target
// the same feature the most recent one wins.
func (r *Repo) ListLiveOverrides(ctx context.Context, tenantID uuid.UUID, now time.Time) (map[string]domain.LiveOverride, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT feature_key, value, expires_at FROM entitlements.tenant_overrides
		 WHERE tenant_id = $1 AND (expires_at IS NULL OR expires_at > $2)
		 ORDER BY created_at`, tenantID, now)
	if err != nil {
		return nil, fmt.Errorf("entitlements: list overrides: %w", err)
	}
	defer rows.Close()

	out := map[string]domain.LiveOverride{}
	for rows.Next() {
		var (
			key       string
			valRaw    []byte
			expiresAt *time.Time
		)
		if err := rows.Scan(&key, &valRaw, &expiresAt); err != nil {
			return nil, fmt.Errorf("entitlements: scan override: %w", err)
		}
		var v any
		if err := json.Unmarshal(valRaw, &v); err != nil {
			return nil, fmt.Errorf("entitlements: unmarshal override: %w", err)
		}
		out[key] = domain.LiveOverride{Value: v, ExpiresAt: expiresAt}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entitlements: iterate overrides: %w", err)
	}
	return out, nil
}

// GetEffective returns the tenant's materialized effective set, keyed by feature.
func (r *Repo) GetEffective(ctx context.Context, tenantID uuid.UUID) (map[string]domain.Resolved, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT feature_key, value, source FROM entitlements.effective_entitlements WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("entitlements: get effective: %w", err)
	}
	defer rows.Close()

	out := map[string]domain.Resolved{}
	for rows.Next() {
		var (
			key    string
			valRaw []byte
			source string
		)
		if err := rows.Scan(&key, &valRaw, &source); err != nil {
			return nil, fmt.Errorf("entitlements: scan effective: %w", err)
		}
		var v any
		if err := json.Unmarshal(valRaw, &v); err != nil {
			return nil, fmt.Errorf("entitlements: unmarshal effective: %w", err)
		}
		out[key] = domain.Resolved{Value: v, Source: source}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entitlements: iterate effective: %w", err)
	}
	return out, nil
}

// ReplaceEffective rewrites the tenant's materialized set: it deletes the current
// rows and inserts the given set within the ambient transaction. Callers invoke
// it only when the set actually changed.
func (r *Repo) ReplaceEffective(ctx context.Context, tenantID uuid.UUID, set map[string]domain.Resolved, now time.Time) error {
	q := platformpg.Q(ctx, r.pool)
	if _, err := q.Exec(ctx, `DELETE FROM entitlements.effective_entitlements WHERE tenant_id = $1`, tenantID); err != nil {
		return fmt.Errorf("entitlements: clear effective: %w", err)
	}
	for key, e := range set {
		val, err := json.Marshal(e.Value)
		if err != nil {
			return fmt.Errorf("entitlements: marshal effective: %w", err)
		}
		if _, err := q.Exec(ctx,
			`INSERT INTO entitlements.effective_entitlements (tenant_id, feature_key, value, source, updated_at)
			 VALUES ($1, $2, $3::jsonb, $4, $5)`,
			tenantID, key, string(val), e.Source, now); err != nil {
			return fmt.Errorf("entitlements: insert effective: %w", err)
		}
	}
	return nil
}

const overrideColumns = `id, tenant_id, feature_key, value, reason, actor, expires_at, created_at`

func scanOverride(row rowScanner) (*domain.Override, error) {
	var (
		o      domain.Override
		valRaw []byte
	)
	if err := row.Scan(&o.ID, &o.TenantID, &o.FeatureKey, &valRaw, &o.Reason, &o.Actor, &o.ExpiresAt, &o.CreatedAt); err != nil {
		return nil, err
	}
	if len(valRaw) > 0 {
		if err := json.Unmarshal(valRaw, &o.Value); err != nil {
			return nil, fmt.Errorf("entitlements: unmarshal override value: %w", err)
		}
	}
	return &o, nil
}

// InsertOverride persists a new tenant override.
func (r *Repo) InsertOverride(ctx context.Context, o *domain.Override) error {
	val, err := json.Marshal(o.Value)
	if err != nil {
		return fmt.Errorf("entitlements: marshal override: %w", err)
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO entitlements.tenant_overrides (id, tenant_id, feature_key, value, reason, actor, expires_at, created_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)`,
		o.ID, o.TenantID, o.FeatureKey, string(val), o.Reason, o.Actor, o.ExpiresAt, o.CreatedAt)
	if err != nil {
		return fmt.Errorf("entitlements: insert override: %w", err)
	}
	return nil
}

// UpdateOverride persists an override's mutable fields (value, reason, actor,
// expiry). The feature key and tenant are immutable.
func (r *Repo) UpdateOverride(ctx context.Context, o *domain.Override) error {
	val, err := json.Marshal(o.Value)
	if err != nil {
		return fmt.Errorf("entitlements: marshal override: %w", err)
	}
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE entitlements.tenant_overrides
			SET value = $2::jsonb, reason = $3, actor = $4, expires_at = $5
		 WHERE id = $1`,
		o.ID, string(val), o.Reason, o.Actor, o.ExpiresAt)
	if err != nil {
		return fmt.Errorf("entitlements: update override: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("override not found")
	}
	return nil
}

// GetOverride returns one override scoped to a tenant.
func (r *Repo) GetOverride(ctx context.Context, tenantID, id uuid.UUID) (*domain.Override, error) {
	row := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT `+overrideColumns+` FROM entitlements.tenant_overrides WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	o, err := scanOverride(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("override not found")
	}
	if err != nil {
		return nil, fmt.Errorf("entitlements: get override: %w", err)
	}
	return o, nil
}

// ListOverrides returns a tenant's overrides (live and expired), newest first.
func (r *Repo) ListOverrides(ctx context.Context, tenantID uuid.UUID) ([]*domain.Override, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT `+overrideColumns+` FROM entitlements.tenant_overrides WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("entitlements: list overrides: %w", err)
	}
	defer rows.Close()

	var out []*domain.Override
	for rows.Next() {
		o, err := scanOverride(rows)
		if err != nil {
			return nil, fmt.Errorf("entitlements: scan override: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entitlements: iterate overrides: %w", err)
	}
	return out, nil
}

// DeleteOverride removes one override scoped to a tenant, returning NotFound when
// no row matched.
func (r *Repo) DeleteOverride(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`DELETE FROM entitlements.tenant_overrides WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("entitlements: delete override: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("override not found")
	}
	return nil
}

// ListExpiredOverrides returns every override whose expires_at has passed as of
// now. The expiry job reverts and removes them.
func (r *Repo) ListExpiredOverrides(ctx context.Context, now time.Time) ([]*domain.Override, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT `+overrideColumns+` FROM entitlements.tenant_overrides
		 WHERE expires_at IS NOT NULL AND expires_at <= $1 ORDER BY tenant_id, created_at`, now)
	if err != nil {
		return nil, fmt.Errorf("entitlements: list expired overrides: %w", err)
	}
	defer rows.Close()

	var out []*domain.Override
	for rows.Next() {
		o, err := scanOverride(rows)
		if err != nil {
			return nil, fmt.Errorf("entitlements: scan expired override: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entitlements: iterate expired overrides: %w", err)
	}
	return out, nil
}
