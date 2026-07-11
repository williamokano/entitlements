package events

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Idempotent wraps h so a given consumer processes each event at most once.
//
// It records (consumer, event_id) in platform.processed_events within the
// ambient delivery transaction before running h: if the row already exists the
// delivery is a duplicate and h is skipped. Because the insert and h share the
// transaction, a failure in h rolls the record back and the event is retried;
// a success commits both together.
func Idempotent(consumer string, pool *pgxpool.Pool, h Handler) Handler {
	return func(ctx context.Context, e Event) error {
		const q = `
			INSERT INTO platform.processed_events (consumer, event_id)
			VALUES ($1, $2)
			ON CONFLICT (consumer, event_id) DO NOTHING`
		tag, err := postgres.Q(ctx, pool).Exec(ctx, q, consumer, e.ID)
		if err != nil {
			return fmt.Errorf("events: mark processed: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Already processed by this consumer; drop the duplicate.
			return nil
		}
		return h(ctx, e)
	}
}
