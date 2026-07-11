// Package domain holds the tenant aggregate and its rules. It is pure: only the
// standard library, uuid, and the platform error taxonomy (enforced by
// depguard).
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

// Status is a tenant's lifecycle state.
type Status string

// Tenant lifecycle states.
const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusDeleted   Status = "deleted"
)

// allowedTransitions maps each status to the states it may move to.
var allowedTransitions = map[Status][]Status{
	StatusActive:    {StatusSuspended, StatusDeleted},
	StatusSuspended: {StatusActive, StatusDeleted},
	StatusDeleted:   {},
}

// Tenant is the root of isolation.
type Tenant struct {
	ID        uuid.UUID
	Slug      string
	Name      string
	Status    Status
	Settings  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New builds an active tenant with a normalized slug.
func New(id uuid.UUID, slug, name string, settings map[string]any, now time.Time) (*Tenant, error) {
	normalized, err := NormalizeSlug(slug)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Validation("tenant name must not be empty")
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return &Tenant{
		ID:        id,
		Slug:      normalized,
		Name:      name,
		Status:    StatusActive,
		Settings:  settings,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// transitionTo moves the tenant to `to` if the transition is allowed.
func (t *Tenant) transitionTo(to Status) error {
	for _, s := range allowedTransitions[t.Status] {
		if s == to {
			t.Status = to
			return nil
		}
	}
	return apperr.Conflict(fmt.Sprintf("cannot transition tenant from %s to %s", t.Status, to))
}

// Suspend moves an active tenant to suspended.
func (t *Tenant) Suspend() error { return t.transitionTo(StatusSuspended) }

// Reactivate moves a suspended tenant back to active.
func (t *Tenant) Reactivate() error { return t.transitionTo(StatusActive) }

// Delete soft-deletes an active or suspended tenant.
func (t *Tenant) Delete() error { return t.transitionTo(StatusDeleted) }

// slugPattern allows lowercase alphanumerics separated by single hyphens, with
// no leading, trailing, or repeated hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	slugMinLen = 3
	slugMaxLen = 63
)

// NormalizeSlug lowercases and trims a slug and validates its shape.
func NormalizeSlug(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) < slugMinLen || len(s) > slugMaxLen {
		return "", apperr.Validation(fmt.Sprintf("slug must be %d-%d characters", slugMinLen, slugMaxLen))
	}
	if !slugPattern.MatchString(s) {
		return "", apperr.Validation("slug may contain only lowercase letters, digits, and single hyphens")
	}
	return s, nil
}

// Repository persists tenants. Reads exclude soft-deleted tenants.
type Repository interface {
	Create(ctx context.Context, t *Tenant) error
	Update(ctx context.Context, t *Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
}
