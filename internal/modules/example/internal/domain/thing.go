// Package domain holds the example module's entities and repository contract.
//
// It is pure: it imports only the standard library and value types (uuid),
// never the service/adapter layers, the platform, or other modules. This purity
// is enforced by depguard.
package domain

import (
	"context"

	"github.com/google/uuid"
)

// Thing is the example aggregate.
type Thing struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Name         string
	ProcessCount int
}

// Repository persists things.
type Repository interface {
	Create(ctx context.Context, t *Thing) error
	Get(ctx context.Context, id uuid.UUID) (*Thing, error)
	IncrementProcessCount(ctx context.Context, id uuid.UUID) error
}
