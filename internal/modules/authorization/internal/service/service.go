// Package service holds the authorization use cases: role CRUD, assignment, and
// the permission check.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authorization/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Service implements the authorization use cases and the ports.Authorizer.
type Service struct {
	uow     *postgres.UnitOfWork
	roles   domain.RoleRepository
	assigns domain.AssignmentRepository
	ids     id.Generator
	clk     clock.Clock
}

// New builds a Service.
func New(uow *postgres.UnitOfWork, roles domain.RoleRepository, assigns domain.AssignmentRepository, ids id.Generator, clk clock.Clock) *Service {
	return &Service{uow: uow, roles: roles, assigns: assigns, ids: ids, clk: clk}
}

// RoleView is a read model of a role.
type RoleView struct {
	ID          uuid.UUID
	Name        string
	Permissions []string
	System      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateRole creates a custom role.
func (s *Service) CreateRole(ctx context.Context, tenantID uuid.UUID, name string, permissions []string) (RoleView, error) {
	role, err := domain.NewRole(s.ids.New(), tenantID, name, permissions, s.clk.Now().UTC())
	if err != nil {
		return RoleView{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		return s.roles.Create(ctx, role)
	}); err != nil {
		return RoleView{}, err
	}
	return toRoleView(role), nil
}

// UpdateRole updates a custom role's name and permissions. System roles are
// immutable.
func (s *Service) UpdateRole(ctx context.Context, tenantID, id uuid.UUID, name string, permissions []string) (RoleView, error) {
	if err := domain.ValidatePermissions(permissions); err != nil {
		return RoleView{}, err
	}
	var view RoleView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		role, err := s.roles.Get(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if role.System {
			return apperr.Conflict("system roles are immutable")
		}
		if name != "" {
			role.Name = name
		}
		if permissions != nil {
			role.Permissions = permissions
		}
		role.UpdatedAt = s.clk.Now().UTC()
		if err := s.roles.Update(ctx, role); err != nil {
			return err
		}
		view = toRoleView(role)
		return nil
	})
	return view, err
}

// DeleteRole deletes a custom role. System roles cannot be deleted.
func (s *Service) DeleteRole(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		role, err := s.roles.Get(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if role.System {
			return apperr.Conflict("system roles cannot be deleted")
		}
		return s.roles.Delete(ctx, tenantID, id)
	})
}

// ListRoles returns a tenant's roles.
func (s *Service) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]RoleView, error) {
	roles, err := s.roles.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	views := make([]RoleView, 0, len(roles))
	for _, r := range roles {
		views = append(views, toRoleView(r))
	}
	return views, nil
}

// GetRole returns one role.
func (s *Service) GetRole(ctx context.Context, tenantID, id uuid.UUID) (RoleView, error) {
	role, err := s.roles.Get(ctx, tenantID, id)
	if err != nil {
		return RoleView{}, err
	}
	return toRoleView(role), nil
}

// Assign grants a role to a user within a tenant (idempotent).
func (s *Service) Assign(ctx context.Context, tenantID, roleID, userID uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if _, err := s.roles.Get(ctx, tenantID, roleID); err != nil {
			return err
		}
		return s.assigns.Assign(ctx, s.ids.New(), tenantID, userID, roleID, s.clk.Now().UTC())
	})
}

// Unassign revokes a role from a user within a tenant.
func (s *Service) Unassign(ctx context.Context, tenantID, roleID, userID uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		return s.assigns.Unassign(ctx, tenantID, userID, roleID)
	})
}

// SeedSystemRoles idempotently creates the owner/admin/member roles for a tenant.
// It is invoked by the provisioning hook when a tenant is created.
func (s *Service) SeedSystemRoles(ctx context.Context, tenantID uuid.UUID) error {
	now := s.clk.Now().UTC()
	for _, seed := range domain.SystemRoleSeeds() {
		role := &domain.Role{
			ID:          s.ids.New(),
			TenantID:    tenantID,
			Name:        seed.Name,
			Permissions: seed.Permissions,
			System:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.roles.Create(ctx, role); err != nil {
			// A duplicate means the role already exists — seeding is idempotent.
			if apperr.KindOf(err) == apperr.KindConflict {
				continue
			}
			return err
		}
	}
	return nil
}

// Check implements ports.Authorizer. A machine principal is checked against its
// API-key scopes; a user principal against the union of its roles' permissions
// in the current tenant. No principal, or no grant, is forbidden.
func (s *Service) Check(ctx context.Context, permission string) error {
	principal, ok := authctx.PrincipalFromContext(ctx)
	if !ok {
		return apperr.Unauthorized("authentication required")
	}

	if principal.Kind == authctx.PrincipalMachine {
		if domain.PermissionsGrant(principal.Scopes, permission) {
			return nil
		}
		return apperr.Forbidden("permission denied")
	}

	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return apperr.Forbidden("permission denied")
	}
	userID, err := uuid.Parse(principal.Subject)
	if err != nil {
		return apperr.Forbidden("permission denied")
	}
	perms, err := s.assigns.PermissionsFor(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if domain.PermissionsGrant(perms, permission) {
		return nil
	}
	return apperr.Forbidden("permission denied")
}

func toRoleView(r *domain.Role) RoleView {
	perms := r.Permissions
	if perms == nil {
		perms = []string{}
	}
	return RoleView{
		ID:          r.ID,
		Name:        r.Name,
		Permissions: perms,
		System:      r.System,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
