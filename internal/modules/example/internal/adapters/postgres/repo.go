// Package postgres is the example module's driven adapter: a Repository backed
// by the platform pgx pool. It routes every query through platform postgres.Q
// so it joins the ambient UnitOfWork transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/example/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

// Repo is a Postgres-backed domain.Repository.
type Repo struct {
	pool *pgxpool.Pool
}

// New builds a Repo.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Create inserts a thing.
func (r *Repo) Create(ctx context.Context, t *domain.Thing) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO example.things (id, tenant_id, name) VALUES ($1, $2, $3)`,
		t.ID, t.TenantID, t.Name)
	if err != nil {
		return fmt.Errorf("example: insert thing: %w", err)
	}
	return nil
}

// Get returns a thing by ID, or apperr.NotFound.
func (r *Repo) Get(ctx context.Context, id uuid.UUID) (*domain.Thing, error) {
	var t domain.Thing
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, tenant_id, name, process_count FROM example.things WHERE id = $1`, id).
		Scan(&t.ID, &t.TenantID, &t.Name, &t.ProcessCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("thing not found")
	}
	if err != nil {
		return nil, fmt.Errorf("example: get thing: %w", err)
	}
	return &t, nil
}

// IncrementProcessCount bumps a thing's process counter by one.
func (r *Repo) IncrementProcessCount(ctx context.Context, id uuid.UUID) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE example.things SET process_count = process_count + 1 WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("example: increment process count: %w", err)
	}
	return nil
}
