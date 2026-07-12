// Package postgres is the subscription module's driven adapter: the subscription
// and transition repository over the platform pgx pool, routing through
// postgres.Q so writes join the ambient UnitOfWork transaction.
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

// Update persists a subscription's mutable fields, including the plan pin and
// any scheduled change.
func (r *Repo) Update(ctx context.Context, s *domain.Subscription) error {
	var pendingPV *uuid.UUID
	var pendingCycle *string
	if s.Scheduled != nil {
		pendingPV = &s.Scheduled.PlanVersionID
		c := string(s.Scheduled.BillingCycle)
		pendingCycle = &c
	}
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE subscription.subscriptions SET status = $2, current_period_start = $3, current_period_end = $4,
			trial_ends_at = $5, cancel_at_period_end = $6, updated_at = $7,
			plan_version_id = $8, billing_cycle = $9, pending_plan_version_id = $10, pending_billing_cycle = $11,
			renewal_emitted_period_end = $12, trial_ending_emitted = $13
		 WHERE id = $1`,
		s.ID, string(s.Status), s.CurrentPeriodStart, s.CurrentPeriodEnd, s.TrialEndsAt, s.CancelAtPeriodEnd, s.UpdatedAt,
		s.PlanVersionID, string(s.BillingCycle), pendingPV, pendingCycle,
		s.RenewalEmittedPeriodEnd, s.TrialEndingEmitted)
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
			t               domain.Transition
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

// subscriptionColumns is the shared SELECT list; scanRow decodes exactly it.
const subscriptionColumns = `id, tenant_id, plan_version_id, billing_cycle, status, current_period_start, current_period_end, trial_ends_at, cancel_at_period_end, pending_plan_version_id, pending_billing_cycle, renewal_emitted_period_end, trial_ending_emitted, created_at, updated_at`

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRow(row rowScanner) (*domain.Subscription, error) {
	var (
		s            domain.Subscription
		cycle        string
		status       string
		pendingPV    *uuid.UUID
		pendingCycle *string
	)
	if err := row.Scan(&s.ID, &s.TenantID, &s.PlanVersionID, &cycle, &status, &s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.TrialEndsAt, &s.CancelAtPeriodEnd, &pendingPV, &pendingCycle, &s.RenewalEmittedPeriodEnd, &s.TrialEndingEmitted,
		&s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.BillingCycle = domain.BillingCycle(cycle)
	s.Status = domain.State(status)
	if pendingPV != nil && pendingCycle != nil {
		s.Scheduled = &domain.ScheduledChange{PlanVersionID: *pendingPV, BillingCycle: domain.BillingCycle(*pendingCycle)}
	}
	return &s, nil
}

func (r *Repo) scanOne(ctx context.Context, where string, args ...any) (*domain.Subscription, error) {
	row := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM subscription.subscriptions `+where, args...)
	s, err := scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("subscription not found")
	}
	if err != nil {
		return nil, fmt.Errorf("subscription: query: %w", err)
	}
	return s, nil
}

func (r *Repo) scanMany(ctx context.Context, where string, args ...any) ([]*domain.Subscription, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT `+subscriptionColumns+` FROM subscription.subscriptions `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("subscription: query: %w", err)
	}
	defer rows.Close()

	var out []*domain.Subscription
	for rows.Next() {
		s, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("subscription: scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("subscription: iterate: %w", err)
	}
	return out, nil
}

// ListRenewable returns live subscriptions whose current period has ended and
// for which a renewal has not yet been emitted for the current period boundary.
func (r *Repo) ListRenewable(ctx context.Context, now time.Time) ([]*domain.Subscription, error) {
	return r.scanMany(ctx,
		`WHERE status IN ('active', 'past_due', 'grace')
		   AND current_period_end <= $1
		   AND (renewal_emitted_period_end IS NULL OR renewal_emitted_period_end < current_period_end)`, now)
}

// ListTrialing returns every trialing subscription (the trial job filters
// further in the domain).
func (r *Repo) ListTrialing(ctx context.Context) ([]*domain.Subscription, error) {
	return r.scanMany(ctx, `WHERE status = 'trialing'`)
}

// GetAddon returns one attached addon by (subscription, addon version).
func (r *Repo) GetAddon(ctx context.Context, subscriptionID, addonVersionID uuid.UUID) (*domain.Addon, error) {
	var a domain.Addon
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, subscription_id, addon_version_id, quantity, created_at, updated_at
		 FROM subscription.subscription_addons WHERE subscription_id = $1 AND addon_version_id = $2`,
		subscriptionID, addonVersionID).
		Scan(&a.ID, &a.SubscriptionID, &a.AddonVersionID, &a.Quantity, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("addon not attached")
	}
	if err != nil {
		return nil, fmt.Errorf("subscription: get addon: %w", err)
	}
	return &a, nil
}

// UpsertAddon inserts an attachment or updates its quantity.
func (r *Repo) UpsertAddon(ctx context.Context, a *domain.Addon) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO subscription.subscription_addons (id, subscription_id, addon_version_id, quantity, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (subscription_id, addon_version_id)
		 DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = EXCLUDED.updated_at`,
		a.ID, a.SubscriptionID, a.AddonVersionID, a.Quantity, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("subscription: upsert addon: %w", err)
	}
	return nil
}

// DeleteAddon removes an attachment.
func (r *Repo) DeleteAddon(ctx context.Context, subscriptionID, addonVersionID uuid.UUID) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`DELETE FROM subscription.subscription_addons WHERE subscription_id = $1 AND addon_version_id = $2`,
		subscriptionID, addonVersionID)
	if err != nil {
		return fmt.Errorf("subscription: delete addon: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("addon not attached")
	}
	return nil
}

// ListAddons returns a subscription's attached addons, oldest first.
func (r *Repo) ListAddons(ctx context.Context, subscriptionID uuid.UUID) ([]*domain.Addon, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, subscription_id, addon_version_id, quantity, created_at, updated_at
		 FROM subscription.subscription_addons WHERE subscription_id = $1 ORDER BY created_at`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("subscription: list addons: %w", err)
	}
	defer rows.Close()

	var out []*domain.Addon
	for rows.Next() {
		var a domain.Addon
		if err := rows.Scan(&a.ID, &a.SubscriptionID, &a.AddonVersionID, &a.Quantity, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("subscription: scan addon: %w", err)
		}
		out = append(out, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("subscription: iterate addons: %w", err)
	}
	return out, nil
}
