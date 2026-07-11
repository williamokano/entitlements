// Package events is the asynchronous backbone: an in-process event bus backed
// by a transactional outbox.
//
// A module Publishes an event through the Outbox inside the same UnitOfWork
// transaction as its business state change, so the event and the change commit
// or roll back together — events are never lost or phantom. A Relay worker
// polls the outbox with FOR UPDATE SKIP LOCKED and dispatches each event to the
// Bus's subscribers at least once. Wrapping a handler with Idempotent makes it
// exactly-once-effective by recording (consumer, event_id) in
// platform.processed_events within the delivery transaction.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidEvent is returned when an event fails envelope validation.
var ErrInvalidEvent = errors.New("events: invalid event")

// Event is the envelope stored in the outbox and delivered to subscribers.
//
// TenantID may be uuid.Nil for platform-wide events that belong to no tenant.
type Event struct {
	ID         uuid.UUID
	OccurredAt time.Time
	TenantID   uuid.UUID
	Module     string
	Type       string
	Payload    json.RawMessage
}

// validate enforces the envelope invariants checked at append time.
func (e Event) validate() error {
	switch {
	case e.ID == uuid.Nil:
		return fmt.Errorf("%w: missing id", ErrInvalidEvent)
	case e.OccurredAt.IsZero():
		return fmt.Errorf("%w: missing occurred_at", ErrInvalidEvent)
	case e.Module == "":
		return fmt.Errorf("%w: missing module", ErrInvalidEvent)
	case e.Type == "":
		return fmt.Errorf("%w: missing type", ErrInvalidEvent)
	case len(e.Payload) == 0:
		return fmt.Errorf("%w: missing payload", ErrInvalidEvent)
	case !json.Valid(e.Payload):
		return fmt.Errorf("%w: payload is not valid JSON", ErrInvalidEvent)
	default:
		return nil
	}
}
