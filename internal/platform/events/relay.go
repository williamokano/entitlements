package events

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Relay delivers outbox events to the Bus at least once.
//
// It processes one event per transaction: it locks the next eligible row with
// FOR UPDATE SKIP LOCKED (so concurrent relays never handle the same event),
// dispatches it, and marks it published — all atomically. If a handler fails,
// the transaction rolls back (leaving no trace of the partial delivery) and the
// event's attempt count and next-attempt time are bumped in a separate
// transaction, so it is retried after a backoff.
type Relay struct {
	pool   *pgxpool.Pool
	uow    *postgres.UnitOfWork
	bus    *Bus
	clk    clock.Clock
	logger *slog.Logger
	cfg    RelayConfig
}

// RelayConfig tunes the relay. Zero fields fall back to sensible defaults.
type RelayConfig struct {
	// BatchSize bounds how many events a single ProcessBatch call handles.
	BatchSize int
	// PollInterval is how often Run polls when the outbox is idle.
	PollInterval time.Duration
	// Backoff maps a (post-increment) attempt count to the delay before the
	// event becomes eligible again.
	Backoff func(attempts int) time.Duration
}

func (c RelayConfig) withDefaults() RelayConfig {
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.Backoff == nil {
		c.Backoff = DefaultBackoff
	}
	return c
}

// DefaultBackoff is exponential (2^attempts seconds) capped at five minutes.
func DefaultBackoff(attempts int) time.Duration {
	const maxBackoff = 5 * time.Minute
	d := time.Second << min(attempts, 8) // cap the shift to avoid overflow
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// NewRelay builds a Relay over pool, dispatching to bus.
func NewRelay(pool *pgxpool.Pool, bus *Bus, clk clock.Clock, logger *slog.Logger, cfg RelayConfig) *Relay {
	return &Relay{
		pool:   pool,
		uow:    postgres.NewUnitOfWork(pool),
		bus:    bus,
		clk:    clk,
		logger: logger,
		cfg:    cfg.withDefaults(),
	}
}

// Run polls the outbox until ctx is canceled. Transient errors are logged and
// retried on the next tick rather than stopping the relay.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if _, err := r.ProcessBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.ErrorContext(ctx, "outbox relay batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessBatch delivers up to BatchSize eligible events and returns how many
// were published. It is exported so tests can drive the relay deterministically.
func (r *Relay) ProcessBatch(ctx context.Context) (int, error) {
	published := 0
	for i := 0; i < r.cfg.BatchSize; i++ {
		fetched, delivered, err := r.processOne(ctx)
		if err != nil {
			return published, err
		}
		if !fetched {
			break // no more eligible events
		}
		if delivered {
			published++
		}
	}
	return published, nil
}

// processOne handles the next eligible event. fetched reports whether a row was
// available; delivered reports whether it was published (vs. failed and
// rescheduled).
func (r *Relay) processOne(ctx context.Context) (fetched, delivered bool, err error) {
	now := r.clk.Now().UTC()

	var evt Event
	var handlerErr error

	txErr := r.uow.Do(ctx, func(ctx context.Context) error {
		e, ok, err := fetchNext(ctx, r.pool, now)
		if err != nil || !ok {
			return err // ok=false with nil err commits an empty tx
		}
		fetched = true
		evt = e

		if derr := r.bus.Dispatch(ctx, e); derr != nil {
			handlerErr = derr
			return derr // roll back the partial delivery
		}
		return markPublished(ctx, r.pool, e.ID, now)
	})

	if !fetched {
		return false, false, txErr
	}
	if handlerErr != nil {
		// The delivery rolled back; reschedule with backoff in a fresh tx.
		if ferr := r.recordFailure(ctx, evt.ID, now, handlerErr); ferr != nil {
			return true, false, ferr
		}
		return true, false, nil
	}
	if txErr != nil {
		return true, false, txErr
	}
	return true, true, nil
}

func (r *Relay) recordFailure(ctx context.Context, eventID uuid.UUID, now time.Time, cause error) error {
	return r.uow.Do(ctx, func(ctx context.Context) error {
		q := postgres.Q(ctx, r.pool)

		var attempts int
		if err := q.QueryRow(ctx,
			`SELECT attempts FROM platform.outbox WHERE id = $1 FOR UPDATE`, eventID).
			Scan(&attempts); err != nil {
			return err
		}
		attempts++
		next := now.Add(r.cfg.Backoff(attempts))

		_, err := q.Exec(ctx,
			`UPDATE platform.outbox
			 SET attempts = $2, next_attempt_at = $3, last_error = $4
			 WHERE id = $1`,
			eventID, attempts, next, truncateError(cause.Error()))
		return err
	})
}

func fetchNext(ctx context.Context, pool *pgxpool.Pool, now time.Time) (Event, bool, error) {
	const q = `
		SELECT id, occurred_at, tenant_id, module, event_type, payload
		FROM platform.outbox
		WHERE published_at IS NULL AND next_attempt_at <= $1
		ORDER BY occurred_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1`

	var (
		e       Event
		payload []byte
	)
	err := postgres.Q(ctx, pool).QueryRow(ctx, q, now).
		Scan(&e.ID, &e.OccurredAt, &e.TenantID, &e.Module, &e.Type, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	e.Payload = payload
	return e, true, nil
}

func markPublished(ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID, now time.Time) error {
	_, err := postgres.Q(ctx, pool).Exec(ctx,
		`UPDATE platform.outbox SET published_at = $2 WHERE id = $1`, eventID, now)
	return err
}

func truncateError(s string) string {
	const maxLen = 1000
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
