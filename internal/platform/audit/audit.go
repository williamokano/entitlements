// Package audit provides an append-only audit-log writer shared by every
// module (who / what / when / why).
//
// Record persists an Entry to platform.audit_log through the ambient
// UnitOfWork transaction, so the audit entry commits or rolls back atomically
// with the change it describes. The table rejects UPDATE and DELETE at the
// database level.
package audit

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

// Entry is a single audit record. Before and After are arbitrary values
// serialized to JSON snapshots; either may be nil.
type Entry struct {
	Actor    string    // user id, api-key id, or "system"
	TenantID uuid.UUID // uuid.Nil for platform-wide actions
	Action   string    // e.g. "tenant.suspended"
	Resource string    // e.g. "tenant/<uuid>"
	Before   any
	After    any
	Reason   string
}

// Writer records audit entries.
type Writer struct {
	pool *pgxpool.Pool
	gen  id.Generator
	clk  clock.Clock
}

// NewWriter builds an audit Writer.
func NewWriter(pool *pgxpool.Pool, gen id.Generator, clk clock.Clock) *Writer {
	return &Writer{pool: pool, gen: gen, clk: clk}
}

// Record writes e to the audit log within the ambient transaction.
func (w *Writer) Record(ctx context.Context, e Entry) error {
	if e.Action == "" {
		return fmt.Errorf("audit: action must not be empty")
	}

	before, err := marshalSnapshot(e.Before)
	if err != nil {
		return fmt.Errorf("audit: marshal before: %w", err)
	}
	after, err := marshalSnapshot(e.After)
	if err != nil {
		return fmt.Errorf("audit: marshal after: %w", err)
	}

	const q = `
		INSERT INTO platform.audit_log
			(id, actor, tenant_id, action, resource, before, after, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9)`
	_, err = postgres.Q(ctx, w.pool).Exec(ctx, q,
		w.gen.New(), e.Actor, e.TenantID, e.Action, e.Resource, before, after, e.Reason, w.clk.Now().UTC())
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

// marshalSnapshot returns nil (SQL NULL) for a nil value, otherwise the JSON
// encoding as a string for the jsonb column.
func marshalSnapshot(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
