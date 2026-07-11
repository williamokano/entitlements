// Package ports is the example module's public surface: the facade other
// modules may call, and the event types it publishes. It is the only package of
// the module importable from outside (the rest lives under internal/).
package ports

import (
	"context"

	"github.com/google/uuid"
)

// EventThingCreated is published when a thing is created.
const EventThingCreated = "example.thing_created"

// ThingCreated is the payload of EventThingCreated.
type ThingCreated struct {
	ThingID  uuid.UUID `json:"thing_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Name     string    `json:"name"`
}

// Reader is the example module's facade for other modules.
type Reader interface {
	GetThingName(ctx context.Context, id uuid.UUID) (string, error)
}
