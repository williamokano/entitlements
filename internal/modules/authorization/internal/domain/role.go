// Package domain holds the authorization aggregates and permission rules. It is
// pure: only the standard library, uuid, and the platform error taxonomy.
package domain

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// System role names, seeded per tenant.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Role is a tenant-scoped named set of permissions.
type Role struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Name        string
	Permissions []string
	System      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewRole builds a custom (non-system) role with validated permissions.
func NewRole(id, tenantID uuid.UUID, name string, permissions []string, now time.Time) (*Role, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, apperr.Validation("role name must not be empty")
	}
	if err := ValidatePermissions(permissions); err != nil {
		return nil, err
	}
	if permissions == nil {
		permissions = []string{}
	}
	return &Role{
		ID:          id,
		TenantID:    tenantID,
		Name:        trimmed,
		Permissions: permissions,
		System:      false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// permissionPattern allows "resource:action" and the "resource:*" wildcard,
// where resource and action are lowercase alphanumerics/underscores. The
// super-permission "*:*" is also allowed.
var permissionPattern = regexp.MustCompile(`^([a-z0-9_]+|\*):([a-z0-9_]+|\*)$`)

// ValidatePermissions checks each permission string's shape.
func ValidatePermissions(permissions []string) error {
	for _, p := range permissions {
		if !permissionPattern.MatchString(p) {
			return apperr.Validation(fmt.Sprintf("invalid permission %q (want resource:action or resource:*)", p))
		}
	}
	return nil
}

// Grants reports whether the granted permission satisfies the required one.
// "resource:action" is granted by an exact match, by "resource:*", or by the
// super-permission "*:*". A grant on a different resource never leaks across.
func Grants(granted, required string) bool {
	if granted == required {
		return true
	}
	gRes, gAct, ok := splitPermission(granted)
	if !ok {
		return false
	}
	rRes, _, ok := splitPermission(required)
	if !ok {
		return false
	}
	if gRes == "*" && gAct == "*" {
		return true // super-permission
	}
	// A resource wildcard grants every action on that same resource.
	return gAct == "*" && gRes == rRes
}

// PermissionsGrant reports whether any of the granted permissions satisfies the
// required permission.
func PermissionsGrant(granted []string, required string) bool {
	for _, g := range granted {
		if Grants(g, required) {
			return true
		}
	}
	return false
}

func splitPermission(p string) (resource, action string, ok bool) {
	resource, action, found := strings.Cut(p, ":")
	if !found || resource == "" || action == "" {
		return "", "", false
	}
	return resource, action, true
}

// SystemRoleSeeds returns the default system roles for a new tenant.
func SystemRoleSeeds() []struct {
	Name        string
	Permissions []string
} {
	return []struct {
		Name        string
		Permissions []string
	}{
		{RoleOwner, []string{"*:*"}},
		{RoleAdmin, []string{"role:read", "role:write", "member:read", "member:write", "apikey:read", "apikey:write"}},
		{RoleMember, []string{"role:read", "member:read"}},
	}
}

// RoleRepository persists roles.
type RoleRepository interface {
	Create(ctx context.Context, r *Role) error
	Get(ctx context.Context, tenantID, id uuid.UUID) (*Role, error)
	GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*Role, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]*Role, error)
	Update(ctx context.Context, r *Role) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

// AssignmentRepository persists role assignments and resolves a user's effective
// permissions in a tenant.
type AssignmentRepository interface {
	Assign(ctx context.Context, id, tenantID, userID, roleID uuid.UUID, now time.Time) error
	Unassign(ctx context.Context, tenantID, userID, roleID uuid.UUID) error
	// PermissionsFor returns the union of permissions across the user's roles in
	// the tenant.
	PermissionsFor(ctx context.Context, tenantID, userID uuid.UUID) ([]string, error)
}
