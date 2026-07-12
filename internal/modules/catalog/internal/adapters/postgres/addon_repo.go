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

// Addons is a Postgres-backed domain.AddonRepository.
type Addons struct {
	pool *pgxpool.Pool
}

// NewAddons builds an Addons repository.
func NewAddons(pool *pgxpool.Pool) *Addons { return &Addons{pool: pool} }

// CreateAddon inserts an addon, mapping a duplicate key to a conflict.
func (r *Addons) CreateAddon(ctx context.Context, a *domain.Addon) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO catalog.addons (id, key, name, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		a.ID, a.Key, a.Name, string(a.Status), a.CreatedAt, a.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("an addon with this key already exists")
		}
		return fmt.Errorf("catalog: insert addon: %w", err)
	}
	return nil
}

// GetAddon returns an addon by id.
func (r *Addons) GetAddon(ctx context.Context, id uuid.UUID) (*domain.Addon, error) {
	var (
		a      domain.Addon
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, key, name, status, created_at, updated_at FROM catalog.addons WHERE id = $1`, id).
		Scan(&a.ID, &a.Key, &a.Name, &status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("addon not found")
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: query addon: %w", err)
	}
	a.Status = domain.AddonStatus(status)
	return &a, nil
}

// UpdateAddon persists an addon's mutable fields.
func (r *Addons) UpdateAddon(ctx context.Context, a *domain.Addon) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE catalog.addons SET name = $2, status = $3, updated_at = $4 WHERE id = $1`,
		a.ID, a.Name, string(a.Status), a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: update addon: %w", err)
	}
	return nil
}

// ListAddons returns all addons, newest first.
func (r *Addons) ListAddons(ctx context.Context) ([]*domain.Addon, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, key, name, status, created_at, updated_at FROM catalog.addons ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list addons: %w", err)
	}
	defer rows.Close()

	var addons []*domain.Addon
	for rows.Next() {
		var (
			a      domain.Addon
			status string
		)
		if err := rows.Scan(&a.ID, &a.Key, &a.Name, &status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("catalog: scan addon: %w", err)
		}
		a.Status = domain.AddonStatus(status)
		addons = append(addons, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate addons: %w", err)
	}
	return addons, nil
}

// AddonVersions is a Postgres-backed domain.AddonVersionRepository.
type AddonVersions struct {
	pool *pgxpool.Pool
}

// NewAddonVersions builds an AddonVersions repository.
func NewAddonVersions(pool *pgxpool.Pool) *AddonVersions { return &AddonVersions{pool: pool} }

// CreateVersion inserts an addon version.
func (r *AddonVersions) CreateVersion(ctx context.Context, v *domain.AddonVersion) error {
	prices, deltas, err := marshalAddonJSON(v)
	if err != nil {
		return err
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO catalog.addon_versions
			(id, addon_id, version, status, currency, prices, quantity_allowed, compatible_plan_keys, deltas, published_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9::jsonb, $10, $11, $12)`,
		v.ID, v.AddonID, v.Version, string(v.Status), v.Currency, string(prices),
		v.QuantityAllowed, v.CompatiblePlanKeys, string(deltas), v.PublishedAt, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a draft version already exists for this addon")
		}
		return fmt.Errorf("catalog: insert addon version: %w", err)
	}
	return nil
}

// GetVersion returns an addon version by id.
func (r *AddonVersions) GetVersion(ctx context.Context, id uuid.UUID) (*domain.AddonVersion, error) {
	return r.scanOne(ctx, `WHERE id = $1`, id)
}

// UpdateVersion persists an addon version's mutable fields.
func (r *AddonVersions) UpdateVersion(ctx context.Context, v *domain.AddonVersion) error {
	prices, deltas, err := marshalAddonJSON(v)
	if err != nil {
		return err
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE catalog.addon_versions SET status = $2, currency = $3, prices = $4::jsonb,
			quantity_allowed = $5, compatible_plan_keys = $6, deltas = $7::jsonb, published_at = $8, updated_at = $9
		 WHERE id = $1`,
		v.ID, string(v.Status), v.Currency, string(prices),
		v.QuantityAllowed, v.CompatiblePlanKeys, string(deltas), v.PublishedAt, v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: update addon version: %w", err)
	}
	return nil
}

// ListVersions returns an addon's versions, highest number first.
func (r *AddonVersions) ListVersions(ctx context.Context, addonID uuid.UUID) ([]*domain.AddonVersion, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx, addonVersionSelect+`WHERE addon_id = $1 ORDER BY version DESC`, addonID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list addon versions: %w", err)
	}
	defer rows.Close()

	var versions []*domain.AddonVersion
	for rows.Next() {
		v, err := scanAddonVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate addon versions: %w", err)
	}
	return versions, nil
}

// GetDraftVersion returns an addon's draft version, if any.
func (r *AddonVersions) GetDraftVersion(ctx context.Context, addonID uuid.UUID) (*domain.AddonVersion, error) {
	return r.scanOne(ctx, `WHERE addon_id = $1 AND status = 'draft'`, addonID)
}

// LatestPublishedVersion returns an addon's most recent published version.
func (r *AddonVersions) LatestPublishedVersion(ctx context.Context, addonID uuid.UUID) (*domain.AddonVersion, error) {
	return r.scanOne(ctx, `WHERE addon_id = $1 AND status = 'published' ORDER BY version DESC LIMIT 1`, addonID)
}

// MaxVersionNumber returns the highest version number for an addon (0 if none).
func (r *AddonVersions) MaxVersionNumber(ctx context.Context, addonID uuid.UUID) (int, error) {
	var maxVersion int
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM catalog.addon_versions WHERE addon_id = $1`, addonID).Scan(&maxVersion)
	if err != nil {
		return 0, fmt.Errorf("catalog: max addon version: %w", err)
	}
	return maxVersion, nil
}

const addonVersionSelect = `SELECT id, addon_id, version, status, currency, prices, quantity_allowed, compatible_plan_keys, deltas, published_at, created_at, updated_at FROM catalog.addon_versions `

func (r *AddonVersions) scanOne(ctx context.Context, where string, args ...any) (*domain.AddonVersion, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx, addonVersionSelect+where, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog: query addon version: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("catalog: query addon version: %w", err)
		}
		return nil, apperr.NotFound("addon version not found")
	}
	return scanAddonVersion(rows)
}

func marshalAddonJSON(v *domain.AddonVersion) (prices, deltas []byte, err error) {
	prices, err = json.Marshal(v.Prices)
	if err != nil {
		return nil, nil, fmt.Errorf("catalog: marshal addon prices: %w", err)
	}
	deltas, err = json.Marshal(v.Deltas)
	if err != nil {
		return nil, nil, fmt.Errorf("catalog: marshal addon deltas: %w", err)
	}
	return prices, deltas, nil
}

func scanAddonVersion(rows pgx.Rows) (*domain.AddonVersion, error) {
	var (
		v      domain.AddonVersion
		status string
		prices []byte
		deltas []byte
	)
	if err := rows.Scan(&v.ID, &v.AddonID, &v.Version, &status, &v.Currency, &prices,
		&v.QuantityAllowed, &v.CompatiblePlanKeys, &deltas, &v.PublishedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, fmt.Errorf("catalog: scan addon version: %w", err)
	}
	v.Status = domain.VersionStatus(status)
	if len(prices) > 0 {
		if err := json.Unmarshal(prices, &v.Prices); err != nil {
			return nil, fmt.Errorf("catalog: unmarshal addon prices: %w", err)
		}
	}
	if v.Prices == nil {
		v.Prices = []domain.Price{}
	}
	if len(deltas) > 0 {
		if err := json.Unmarshal(deltas, &v.Deltas); err != nil {
			return nil, fmt.Errorf("catalog: unmarshal addon deltas: %w", err)
		}
	}
	if v.Deltas == nil {
		v.Deltas = []domain.Delta{}
	}
	if v.CompatiblePlanKeys == nil {
		v.CompatiblePlanKeys = []string{}
	}
	return &v, nil
}
