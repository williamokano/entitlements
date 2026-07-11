package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Outbox appends events to platform.outbox. Append/Publish run through the
// ambient UnitOfWork transaction (via postgres.Q), so an event is committed
// atomically with the business change that produced it.
type Outbox struct {
	pool *pgxpool.Pool
	gen  id.Generator
	clk  clock.Clock
}

// NewOutbox builds an Outbox that stamps event IDs with gen and timestamps
// with clk.
func NewOutbox(pool *pgxpool.Pool, gen id.Generator, clk clock.Clock) *Outbox {
	return &Outbox{pool: pool, gen: gen, clk: clk}
}

// EventInput describes an event to publish; the Outbox stamps the ID and
// occurred-at timestamp.
type EventInput struct {
	TenantID uuid.UUID // uuid.Nil for platform-wide events
	Module   string
	Type     string
	Payload  any // marshaled to JSON
}

// Publish marshals in.Payload, stamps a new event, and appends it to the
// outbox within the ambient transaction. It returns the stored event.
func (o *Outbox) Publish(ctx context.Context, in EventInput) (Event, error) {
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return Event{}, fmt.Errorf("events: marshal payload: %w", err)
	}
	e := Event{
		ID:         o.gen.New(),
		OccurredAt: o.clk.Now().UTC(),
		TenantID:   in.TenantID,
		Module:     in.Module,
		Type:       in.Type,
		Payload:    payload,
	}
	if err := o.Append(ctx, e); err != nil {
		return Event{}, err
	}
	return e, nil
}

// Append validates and writes a fully-formed event to the outbox within the
// ambient transaction.
func (o *Outbox) Append(ctx context.Context, e Event) error {
	if err := e.validate(); err != nil {
		return err
	}
	const q = `
		INSERT INTO platform.outbox
			(id, occurred_at, tenant_id, module, event_type, payload, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $2)`
	_, err := postgres.Q(ctx, o.pool).Exec(ctx, q,
		e.ID, e.OccurredAt, e.TenantID, e.Module, e.Type, string(e.Payload))
	if err != nil {
		return fmt.Errorf("events: append to outbox: %w", err)
	}
	return nil
}
