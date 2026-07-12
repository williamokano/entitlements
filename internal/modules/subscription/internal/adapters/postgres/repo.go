// Package postgres is the subscription module's driven adapter: the subscription
// and transition repository over the platform pgx pool, routing through
// postgres.Q so writes join the ambient UnitOfWork transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

const uniqueViolation = "23505"

// Repo is a Postgres-backed domain.Repository.
type Repo struct {
	pool *pgxpool.Pool
}

// New builds a Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create inserts a subscription, mapping the one-live-per-tenant violation to a
// conflict.
func (r *Repo) Create(ctx context.Context, s *domain.Subscription) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO subscription.subscriptions
			(id, tenant_id, plan_version_id, billing_cycle, status, current_period_start, current_period_end, trial_ends_at, cancel_at_period_end, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		s.ID, s.TenantID, s.PlanVersionID, string(s.BillingCycle), string(s.Status),
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.TrialEndsAt, s.CancelAtPeriodEnd, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("tenant already has a live subscription")
		}
		return fmt.Errorf("subscription: insert: %w", err)
	}
	return nil
}

// Update persists a subscription's mutable fields.
func (r *Repo) Update(ctx context.Context, s *domain.Subscription) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE subscription.subscriptions SET status = $2, current_period_start = $3, current_period_end = $4,
			trial_ends_at = $5, cancel_at_period_end = $6, updated_at = $7 WHERE id = $1`,
		s.ID, string(s.Status), s.CurrentPeriodStart, s.CurrentPeriodEnd, s.TrialEndsAt, s.CancelAtPeriodEnd, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("subscription: update: %w", err)
	}
	return nil
}

// GetByID returns a subscription by id.
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Subscription, error) {
	return r.scanOne(ctx, `WHERE id = $1`, id)
}

// GetLiveForTenant returns a tenant's non-terminal subscription.
func (r *Repo) GetLiveForTenant(ctx context.Context, tenantID uuid.UUID) (*domain.Subscription, error) {
	return r.scanOne(ctx, `WHERE tenant_id = $1 AND status NOT IN ('canceled', 'expired')`, tenantID)
}

// AppendTransition records one state change.
func (r *Repo) AppendTransition(ctx context.Context, t *domain.Transition) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO subscription.subscription_transitions (id, subscription_id, from_state, to_state, event, reason, actor, at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		t.ID, t.SubscriptionID, string(t.From), string(t.To), string(t.Event), t.Reason, t.Actor, t.At)
	if err != nil {
		return fmt.Errorf("subscription: append transition: %w", err)
	}
	return nil
}

// ListTransitions returns a subscription's transition history, oldest first.
func (r *Repo) ListTransitions(ctx context.Context, subscriptionID uuid.UUID) ([]*domain.Transition, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, subscription_id, from_state, to_state, event, reason, actor, at
		 FROM subscription.subscription_transitions WHERE subscription_id = $1 ORDER BY at`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("subscription: list transitions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Transition
	for rows.Next() {
		var (
			t                domain.Transition
			from, to, event string
		)
		if err := rows.Scan(&t.ID, &t.SubscriptionID, &from, &to, &event, &t.Reason, &t.Actor, &t.At); err != nil {
			return nil, fmt.Errorf("subscription: scan transition: %w", err)
		}
		t.From, t.To, t.Event = domain.State(from), domain.State(to), domain.Event(event)
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("subscription: iterate transitions: %w", err)
	}
	return out, nil
}

func (r *Repo) scanOne(ctx context.Context, where string, args ...any) (*domain.Subscription, error) {
	var (
		s      domain.Subscription
		cycle  string
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, tenant_id, plan_version_id, billing_cycle, status, current_period_start, current_period_end, trial_ends_at, cancel_at_period_end, created_at, updated_at
		 FROM subscription.subscriptions `+where, args...).
		Scan(&s.ID, &s.TenantID, &s.PlanVersionID, &cycle, &status, &s.CurrentPeriodStart, &s.CurrentPeriodEnd, &s.TrialEndsAt, &s.CancelAtPeriodEnd, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("subscription not found")
	}
	if err != nil {
		return nil, fmt.Errorf("subscription: query: %w", err)
	}
	s.BillingCycle = domain.BillingCycle(cycle)
	s.Status = domain.State(status)
	return &s, nil
}
