// Package postgres is the catalog module's driven adapter: plan and version
// repositories over the platform pgx pool, routing through postgres.Q so they
// join the ambient UnitOfWork transaction.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/catalog/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

const uniqueViolation = "23505"

// Plans is a Postgres-backed domain.PlanRepository.
type Plans struct {
	pool *pgxpool.Pool
}

// NewPlans builds a Plans repository.
func NewPlans(pool *pgxpool.Pool) *Plans { return &Plans{pool: pool} }

// CreatePlan inserts a plan, mapping a duplicate key to a conflict.
func (r *Plans) CreatePlan(ctx context.Context, p *domain.Plan) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO catalog.plans (id, key, name, status, public, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.ID, p.Key, p.Name, string(p.Status), p.Public, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a plan with this key already exists")
		}
		return fmt.Errorf("catalog: insert plan: %w", err)
	}
	return nil
}

// GetPlan returns a plan by id.
func (r *Plans) GetPlan(ctx context.Context, id uuid.UUID) (*domain.Plan, error) {
	return r.scanOne(ctx, `WHERE id = $1`, id)
}

// UpdatePlan persists a plan's mutable fields.
func (r *Plans) UpdatePlan(ctx context.Context, p *domain.Plan) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE catalog.plans SET name = $2, status = $3, public = $4, updated_at = $5 WHERE id = $1`,
		p.ID, p.Name, string(p.Status), p.Public, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: update plan: %w", err)
	}
	return nil
}

// ListPlans returns all plans, newest first.
func (r *Plans) ListPlans(ctx context.Context) ([]*domain.Plan, error) {
	return r.scanMany(ctx, `ORDER BY created_at DESC`)
}

// ListPublicPlans returns active, public plans.
func (r *Plans) ListPublicPlans(ctx context.Context) ([]*domain.Plan, error) {
	return r.scanMany(ctx, `WHERE status = 'active' AND public ORDER BY created_at DESC`)
}

func (r *Plans) scanOne(ctx context.Context, where string, args ...any) (*domain.Plan, error) {
	var (
		p      domain.Plan
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, key, name, status, public, created_at, updated_at FROM catalog.plans `+where, args...).
		Scan(&p.ID, &p.Key, &p.Name, &status, &p.Public, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("plan not found")
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: query plan: %w", err)
	}
	p.Status = domain.PlanStatus(status)
	return &p, nil
}

func (r *Plans) scanMany(ctx context.Context, tail string) ([]*domain.Plan, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, key, name, status, public, created_at, updated_at FROM catalog.plans `+tail)
	if err != nil {
		return nil, fmt.Errorf("catalog: list plans: %w", err)
	}
	defer rows.Close()

	var plans []*domain.Plan
	for rows.Next() {
		var (
			p      domain.Plan
			status string
		)
		if err := rows.Scan(&p.ID, &p.Key, &p.Name, &status, &p.Public, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("catalog: scan plan: %w", err)
		}
		p.Status = domain.PlanStatus(status)
		plans = append(plans, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate plans: %w", err)
	}
	return plans, nil
}

// Versions is a Postgres-backed domain.PlanVersionRepository.
type Versions struct {
	pool *pgxpool.Pool
}

// NewVersions builds a Versions repository.
func NewVersions(pool *pgxpool.Pool) *Versions { return &Versions{pool: pool} }

// CreateVersion inserts a version, mapping the single-draft/version-number
// constraints to a conflict.
func (r *Versions) CreateVersion(ctx context.Context, v *domain.PlanVersion) error {
	prices, err := json.Marshal(v.Prices)
	if err != nil {
		return fmt.Errorf("catalog: marshal prices: %w", err)
	}
	grants, err := json.Marshal(v.FeatureGrants)
	if err != nil {
		return fmt.Errorf("catalog: marshal grants: %w", err)
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO catalog.plan_versions
			(id, plan_id, version, status, currency, prices, trial_enabled, trial_days, card_required, grace_days, feature_grants, published_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11::jsonb, $12, $13, $14)`,
		v.ID, v.PlanID, v.Version, string(v.Status), v.Currency, string(prices),
		v.Trial.Enabled, v.Trial.Days, v.Trial.CardRequired, v.GraceDays, string(grants),
		v.PublishedAt, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a draft version already exists for this plan")
		}
		return fmt.Errorf("catalog: insert version: %w", err)
	}
	return nil
}

// GetVersion returns a version by id.
func (r *Versions) GetVersion(ctx context.Context, id uuid.UUID) (*domain.PlanVersion, error) {
	return r.scanOne(ctx, `WHERE id = $1`, id)
}

// UpdateVersion persists a version's mutable fields.
func (r *Versions) UpdateVersion(ctx context.Context, v *domain.PlanVersion) error {
	prices, err := json.Marshal(v.Prices)
	if err != nil {
		return fmt.Errorf("catalog: marshal prices: %w", err)
	}
	grants, err := json.Marshal(v.FeatureGrants)
	if err != nil {
		return fmt.Errorf("catalog: marshal grants: %w", err)
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE catalog.plan_versions SET status = $2, currency = $3, prices = $4::jsonb,
			trial_enabled = $5, trial_days = $6, card_required = $7, grace_days = $8,
			feature_grants = $9::jsonb, published_at = $10, updated_at = $11
		 WHERE id = $1`,
		v.ID, string(v.Status), v.Currency, string(prices),
		v.Trial.Enabled, v.Trial.Days, v.Trial.CardRequired, v.GraceDays, string(grants),
		v.PublishedAt, v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: update version: %w", err)
	}
	return nil
}

// ListVersions returns a plan's versions, highest number first.
func (r *Versions) ListVersions(ctx context.Context, planID uuid.UUID) ([]*domain.PlanVersion, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx, versionSelect+`WHERE plan_id = $1 ORDER BY version DESC`, planID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list versions: %w", err)
	}
	defer rows.Close()

	var versions []*domain.PlanVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate versions: %w", err)
	}
	return versions, nil
}

// GetDraftVersion returns a plan's draft version, if any.
func (r *Versions) GetDraftVersion(ctx context.Context, planID uuid.UUID) (*domain.PlanVersion, error) {
	return r.scanOne(ctx, `WHERE plan_id = $1 AND status = 'draft'`, planID)
}

// LatestPublishedVersion returns a plan's most recent published version.
func (r *Versions) LatestPublishedVersion(ctx context.Context, planID uuid.UUID) (*domain.PlanVersion, error) {
	return r.scanOne(ctx, `WHERE plan_id = $1 AND status = 'published' ORDER BY version DESC LIMIT 1`, planID)
}

// MaxVersionNumber returns the highest version number for a plan (0 if none).
func (r *Versions) MaxVersionNumber(ctx context.Context, planID uuid.UUID) (int, error) {
	var maxVersion int
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM catalog.plan_versions WHERE plan_id = $1`, planID).Scan(&maxVersion)
	if err != nil {
		return 0, fmt.Errorf("catalog: max version number: %w", err)
	}
	return maxVersion, nil
}

const versionSelect = `SELECT id, plan_id, version, status, currency, prices, trial_enabled, trial_days, card_required, grace_days, feature_grants, published_at, created_at, updated_at FROM catalog.plan_versions `

func (r *Versions) scanOne(ctx context.Context, where string, args ...any) (*domain.PlanVersion, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx, versionSelect+where, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog: query version: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("catalog: query version: %w", err)
		}
		return nil, apperr.NotFound("plan version not found")
	}
	return scanVersion(rows)
}

func scanVersion(rows pgx.Rows) (*domain.PlanVersion, error) {
	var (
		v      domain.PlanVersion
		status string
		prices []byte
		grants []byte
	)
	if err := rows.Scan(&v.ID, &v.PlanID, &v.Version, &status, &v.Currency, &prices,
		&v.Trial.Enabled, &v.Trial.Days, &v.Trial.CardRequired, &v.GraceDays, &grants,
		&v.PublishedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, fmt.Errorf("catalog: scan version: %w", err)
	}
	v.Status = domain.VersionStatus(status)
	if len(prices) > 0 {
		if err := json.Unmarshal(prices, &v.Prices); err != nil {
			return nil, fmt.Errorf("catalog: unmarshal prices: %w", err)
		}
	}
	if v.Prices == nil {
		v.Prices = []domain.Price{}
	}
	if len(grants) > 0 {
		if err := json.Unmarshal(grants, &v.FeatureGrants); err != nil {
			return nil, fmt.Errorf("catalog: unmarshal grants: %w", err)
		}
	}
	if v.FeatureGrants == nil {
		v.FeatureGrants = map[string]any{}
	}
	return &v, nil
}
