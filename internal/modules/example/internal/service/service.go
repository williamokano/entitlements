// Package service holds the example module's use cases. It owns the transaction
// boundary: creating a thing and publishing its event happen atomically.
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/example/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/example/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Service implements the example use cases and the ports.Reader facade.
type Service struct {
	uow    *postgres.UnitOfWork
	outbox *events.Outbox
	repo   domain.Repository
	ids    id.Generator
	clk    clock.Clock
}

// New builds a Service.
func New(uow *postgres.UnitOfWork, outbox *events.Outbox, repo domain.Repository, ids id.Generator, clk clock.Clock) *Service {
	return &Service{uow: uow, outbox: outbox, repo: repo, ids: ids, clk: clk}
}

// CreateThing creates a thing and publishes example.thing_created in the same
// transaction, so the row and the event commit together.
func (s *Service) CreateThing(ctx context.Context, tenantID uuid.UUID, name string) (*domain.Thing, error) {
	thing := &domain.Thing{ID: s.ids.New(), TenantID: tenantID, Name: name}
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, thing); err != nil {
			return err
		}
		_, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenantID,
			Module:   "example",
			Type:     ports.EventThingCreated,
			Payload:  ports.ThingCreated{ThingID: thing.ID, TenantID: tenantID, Name: name},
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("example: create thing: %w", err)
	}
	return thing, nil
}

// MarkProcessed increments a thing's process counter. It is the effect of
// consuming example.thing_created.
func (s *Service) MarkProcessed(ctx context.Context, id uuid.UUID) error {
	return s.repo.IncrementProcessCount(ctx, id)
}

// GetThingName implements ports.Reader.
func (s *Service) GetThingName(ctx context.Context, id uuid.UUID) (string, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return t.Name, nil
}
