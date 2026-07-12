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

	"github.com/williamokano/entitlements/internal/modules/tenant/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

// Memberships is a Postgres-backed domain.MembershipRepository.
type Memberships struct {
	pool *pgxpool.Pool
}

// NewMemberships builds a Memberships repository.
func NewMemberships(pool *pgxpool.Pool) *Memberships { return &Memberships{pool: pool} }

// CreateMembership inserts a membership, mapping the (tenant, user) uniqueness
// violation to a conflict.
func (r *Memberships) CreateMembership(ctx context.Context, m *domain.Membership) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO tenant.memberships (id, tenant_id, user_id, role, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		m.ID, m.TenantID, m.UserID, m.Role, string(m.Status), m.CreatedAt, m.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("user is already a member of this tenant")
		}
		return fmt.Errorf("tenant: insert membership: %w", err)
	}
	return nil
}

// GetMembership returns an active membership.
func (r *Memberships) GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*domain.Membership, error) {
	var (
		m      domain.Membership
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, tenant_id, user_id, role, status, created_at, updated_at
		 FROM tenant.memberships WHERE tenant_id = $1 AND user_id = $2 AND status = 'active'`, tenantID, userID).
		Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &status, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("membership not found")
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: query membership: %w", err)
	}
	m.Status = domain.MemberStatus(status)
	return &m, nil
}

// ListMembers returns a tenant's active members, newest first.
func (r *Memberships) ListMembers(ctx context.Context, tenantID uuid.UUID) ([]*domain.Membership, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, tenant_id, user_id, role, status, created_at, updated_at
		 FROM tenant.memberships WHERE tenant_id = $1 AND status = 'active' ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant: list members: %w", err)
	}
	defer rows.Close()

	var members []*domain.Membership
	for rows.Next() {
		var (
			m      domain.Membership
			status string
		)
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("tenant: scan member: %w", err)
		}
		m.Status = domain.MemberStatus(status)
		members = append(members, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tenant: iterate members: %w", err)
	}
	return members, nil
}

// RemoveMembership marks an active membership removed.
func (r *Memberships) RemoveMembership(ctx context.Context, tenantID, userID uuid.UUID, at time.Time) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE tenant.memberships SET status = 'removed', updated_at = $3
		 WHERE tenant_id = $1 AND user_id = $2 AND status = 'active'`, tenantID, userID, at)
	if err != nil {
		return fmt.Errorf("tenant: remove membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("membership not found")
	}
	return nil
}

// Invitations is a Postgres-backed domain.InvitationRepository.
type Invitations struct {
	pool *pgxpool.Pool
}

// NewInvitations builds an Invitations repository.
func NewInvitations(pool *pgxpool.Pool) *Invitations { return &Invitations{pool: pool} }

// CreateInvitation inserts an invitation, mapping a duplicate pending invitation
// to a conflict.
func (r *Invitations) CreateInvitation(ctx context.Context, i *domain.Invitation) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO tenant.invitations (id, tenant_id, email, role, status, invited_by, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		i.ID, i.TenantID, i.Email, i.Role, string(i.Status), i.InvitedBy, i.ExpiresAt, i.CreatedAt, i.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a pending invitation for this email already exists")
		}
		return fmt.Errorf("tenant: insert invitation: %w", err)
	}
	return nil
}

// GetInvitation returns an invitation by tenant and id, any status.
func (r *Invitations) GetInvitation(ctx context.Context, tenantID, id uuid.UUID) (*domain.Invitation, error) {
	var (
		inv    domain.Invitation
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, tenant_id, email, role, status, invited_by, expires_at, created_at, updated_at
		 FROM tenant.invitations WHERE tenant_id = $1 AND id = $2`, tenantID, id).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &status, &inv.InvitedBy, &inv.ExpiresAt, &inv.CreatedAt, &inv.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("invitation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: query invitation: %w", err)
	}
	inv.Status = domain.InvitationStatus(status)
	return &inv, nil
}

// ListPendingInvitations returns a tenant's pending invitations, newest first.
func (r *Invitations) ListPendingInvitations(ctx context.Context, tenantID uuid.UUID) ([]*domain.Invitation, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, tenant_id, email, role, status, invited_by, expires_at, created_at, updated_at
		 FROM tenant.invitations WHERE tenant_id = $1 AND status = 'pending' ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant: list invitations: %w", err)
	}
	defer rows.Close()

	var invs []*domain.Invitation
	for rows.Next() {
		var (
			inv    domain.Invitation
			status string
		)
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &status, &inv.InvitedBy, &inv.ExpiresAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			return nil, fmt.Errorf("tenant: scan invitation: %w", err)
		}
		inv.Status = domain.InvitationStatus(status)
		invs = append(invs, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tenant: iterate invitations: %w", err)
	}
	return invs, nil
}

// UpdateInvitation persists mutable invitation fields.
func (r *Invitations) UpdateInvitation(ctx context.Context, i *domain.Invitation) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE tenant.invitations SET status = $3, expires_at = $4, updated_at = $5
		 WHERE tenant_id = $1 AND id = $2`,
		i.TenantID, i.ID, string(i.Status), i.ExpiresAt, i.UpdatedAt)
	if err != nil {
		return fmt.Errorf("tenant: update invitation: %w", err)
	}
	return nil
}
