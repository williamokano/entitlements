// Package postgres is the tenant module's driven adapter: a domain.Repository
// backed by the platform pgx pool, routing queries through platform postgres.Q
// so they join the ambient UnitOfWork transaction.
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

	"github.com/williamokano/entitlements/internal/modules/tenant/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

// uniqueViolation is the Postgres SQLSTATE for a unique-constraint violation.
const uniqueViolation = "23505"

// Repo is a Postgres-backed domain.Repository.
type Repo struct {
	pool *pgxpool.Pool
}

// New builds a Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create inserts a tenant, mapping a slug uniqueness violation to a conflict.
func (r *Repo) Create(ctx context.Context, t *domain.Tenant) error {
	settings, err := json.Marshal(t.Settings)
	if err != nil {
		return fmt.Errorf("tenant: marshal settings: %w", err)
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO tenant.tenants (id, slug, name, status, settings, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
		t.ID, t.Slug, t.Name, string(t.Status), string(settings), t.CreatedAt, t.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("tenant slug already taken")
		}
		return fmt.Errorf("tenant: insert: %w", err)
	}
	return nil
}

// Update persists mutable fields of a tenant.
func (r *Repo) Update(ctx context.Context, t *domain.Tenant) error {
	settings, err := json.Marshal(t.Settings)
	if err != nil {
		return fmt.Errorf("tenant: marshal settings: %w", err)
	}
	_, err = platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE tenant.tenants SET name = $2, status = $3, settings = $4::jsonb, updated_at = $5 WHERE id = $1`,
		t.ID, t.Name, string(t.Status), string(settings), t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("tenant: update: %w", err)
	}
	return nil
}

// GetByID returns a non-deleted tenant by ID.
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return r.getOne(ctx, `WHERE id = $1 AND status <> 'deleted'`, id)
}

// GetBySlug returns a non-deleted tenant by slug.
func (r *Repo) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return r.getOne(ctx, `WHERE slug = $1 AND status <> 'deleted'`, slug)
}

func (r *Repo) getOne(ctx context.Context, where string, arg any) (*domain.Tenant, error) {
	var (
		t        domain.Tenant
		status   string
		settings []byte
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, slug, name, status, settings, created_at, updated_at FROM tenant.tenants `+where, arg).
		Scan(&t.ID, &t.Slug, &t.Name, &status, &settings, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: query: %w", err)
	}
	t.Status = domain.Status(status)
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &t.Settings); err != nil {
			return nil, fmt.Errorf("tenant: unmarshal settings: %w", err)
		}
	}
	if t.Settings == nil {
		t.Settings = map[string]any{}
	}
	return &t, nil
}
