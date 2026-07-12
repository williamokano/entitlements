// Package postgres is the authorization module's driven adapter: role and
// assignment repositories over the platform pgx pool, routing through
// postgres.Q so they join the ambient UnitOfWork transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/authorization/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

const uniqueViolation = "23505"

// Roles is a Postgres-backed domain.RoleRepository.
type Roles struct {
	pool *pgxpool.Pool
}

// NewRoles builds a Roles repository.
func NewRoles(pool *pgxpool.Pool) *Roles { return &Roles{pool: pool} }

// Create inserts a role, mapping a duplicate (tenant, name) to a conflict.
func (r *Roles) Create(ctx context.Context, role *domain.Role) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authz.roles (id, tenant_id, name, permissions, system, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		role.ID, role.TenantID, role.Name, role.Permissions, role.System, role.CreatedAt, role.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a role with this name already exists")
		}
		return fmt.Errorf("authz: insert role: %w", err)
	}
	return nil
}

// Get returns a role by tenant and id.
func (r *Roles) Get(ctx context.Context, tenantID, id uuid.UUID) (*domain.Role, error) {
	return r.scanOne(ctx, `WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

// GetByName returns a role by tenant and name.
func (r *Roles) GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*domain.Role, error) {
	return r.scanOne(ctx, `WHERE tenant_id = $1 AND name = $2`, tenantID, name)
}

func (r *Roles) scanOne(ctx context.Context, where string, args ...any) (*domain.Role, error) {
	var role domain.Role
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, tenant_id, name, permissions, system, created_at, updated_at FROM authz.roles `+where, args...).
		Scan(&role.ID, &role.TenantID, &role.Name, &role.Permissions, &role.System, &role.CreatedAt, &role.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("role not found")
	}
	if err != nil {
		return nil, fmt.Errorf("authz: query role: %w", err)
	}
	return &role, nil
}

// List returns a tenant's roles, system roles first then by name.
func (r *Roles) List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Role, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, tenant_id, name, permissions, system, created_at, updated_at
		 FROM authz.roles WHERE tenant_id = $1 ORDER BY system DESC, name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("authz: list roles: %w", err)
	}
	defer rows.Close()

	var roles []*domain.Role
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(&role.ID, &role.TenantID, &role.Name, &role.Permissions, &role.System, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("authz: scan role: %w", err)
		}
		roles = append(roles, &role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authz: iterate roles: %w", err)
	}
	return roles, nil
}

// Update persists a role's mutable fields.
func (r *Roles) Update(ctx context.Context, role *domain.Role) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authz.roles SET name = $3, permissions = $4, updated_at = $5 WHERE tenant_id = $1 AND id = $2`,
		role.TenantID, role.ID, role.Name, role.Permissions, role.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a role with this name already exists")
		}
		return fmt.Errorf("authz: update role: %w", err)
	}
	return nil
}

// Delete removes a role.
func (r *Roles) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`DELETE FROM authz.roles WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return fmt.Errorf("authz: delete role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("role not found")
	}
	return nil
}

// Assignments is a Postgres-backed domain.AssignmentRepository.
type Assignments struct {
	pool *pgxpool.Pool
}

// NewAssignments builds an Assignments repository.
func NewAssignments(pool *pgxpool.Pool) *Assignments { return &Assignments{pool: pool} }

// Assign grants a role to a user (idempotent: a duplicate assignment is a no-op).
func (r *Assignments) Assign(ctx context.Context, id, tenantID, userID, roleID uuid.UUID, now time.Time) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authz.role_assignments (id, tenant_id, user_id, role_id, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, user_id, role_id) DO NOTHING`,
		id, tenantID, userID, roleID, now)
	if err != nil {
		return fmt.Errorf("authz: assign role: %w", err)
	}
	return nil
}

// Unassign revokes a role from a user.
func (r *Assignments) Unassign(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`DELETE FROM authz.role_assignments WHERE tenant_id = $1 AND user_id = $2 AND role_id = $3`,
		tenantID, userID, roleID)
	if err != nil {
		return fmt.Errorf("authz: unassign role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("assignment not found")
	}
	return nil
}

// PermissionsFor returns the union of permissions across a user's roles in a
// tenant.
func (r *Assignments) PermissionsFor(ctx context.Context, tenantID, userID uuid.UUID) ([]string, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT DISTINCT unnest(ro.permissions)
		 FROM authz.role_assignments ra
		 JOIN authz.roles ro ON ro.id = ra.role_id
		 WHERE ra.tenant_id = $1 AND ra.user_id = $2`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("authz: permissions for user: %w", err)
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("authz: scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authz: iterate permissions: %w", err)
	}
	return perms, nil
}
