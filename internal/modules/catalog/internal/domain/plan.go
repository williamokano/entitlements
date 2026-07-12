// Package domain holds the catalog aggregates and rules. It is pure: only the
// standard library, uuid, and the platform error taxonomy.
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

// PlanStatus is a plan's lifecycle state.
type PlanStatus string

// Plan lifecycle states.
const (
	PlanDraft    PlanStatus = "draft"
	PlanActive   PlanStatus = "active"
	PlanArchived PlanStatus = "archived"
)

// allowedPlanTransitions maps each status to the states it may move to.
var allowedPlanTransitions = map[PlanStatus][]PlanStatus{
	PlanDraft:    {PlanActive, PlanArchived},
	PlanActive:   {PlanArchived},
	PlanArchived: {},
}

// Plan is the catalog's product container. Its pricing/features live in
// versions; a published version is immutable.
type Plan struct {
	ID        uuid.UUID
	Key       string
	Name      string
	Status    PlanStatus
	Public    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// keyPattern allows lowercase alphanumerics separated by single hyphens.
var keyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// NewPlan builds a draft, non-public plan with a validated key.
func NewPlan(id uuid.UUID, key, name string, now time.Time) (*Plan, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	if !keyPattern.MatchString(k) {
		return nil, apperr.Validation("plan key must be lowercase alphanumerics and single hyphens")
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Validation("plan name must not be empty")
	}
	return &Plan{
		ID:        id,
		Key:       k,
		Name:      name,
		Status:    PlanDraft,
		Public:    false,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// transitionTo moves the plan to `to` if allowed.
func (p *Plan) transitionTo(to PlanStatus) error {
	for _, s := range allowedPlanTransitions[p.Status] {
		if s == to {
			p.Status = to
			return nil
		}
	}
	return apperr.Conflict(fmt.Sprintf("cannot transition plan from %s to %s", p.Status, to))
}

// Activate moves a draft plan to active (done when its first version publishes).
func (p *Plan) Activate() error { return p.transitionTo(PlanActive) }

// Archive moves a plan to archived.
func (p *Plan) Archive() error { return p.transitionTo(PlanArchived) }

// PlanRepository persists plans.
type PlanRepository interface {
	CreatePlan(ctx context.Context, p *Plan) error
	GetPlan(ctx context.Context, id uuid.UUID) (*Plan, error)
	UpdatePlan(ctx context.Context, p *Plan) error
	ListPlans(ctx context.Context) ([]*Plan, error)
	ListPublicPlans(ctx context.Context) ([]*Plan, error)
}
