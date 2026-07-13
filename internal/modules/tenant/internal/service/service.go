// Package service holds the tenant use cases. Each mutation and its published
// event share one transaction (via the outbox), so state and event commit
// together.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Service implements the tenant use cases and the ports.TenantReader facade.
type Service struct {
	uow     *postgres.UnitOfWork
	outbox  *events.Outbox
	repo    domain.Repository
	members domain.MembershipRepository
	ids     id.Generator
	clk     clock.Clock
}

// New builds a Service.
func New(uow *postgres.UnitOfWork, outbox *events.Outbox, repo domain.Repository, members domain.MembershipRepository, ids id.Generator, clk clock.Clock) *Service {
	return &Service{uow: uow, outbox: outbox, repo: repo, members: members, ids: ids, clk: clk}
}

// Creator is the authenticated user creating a tenant.
//
// It is optional: tenant creation is an open endpoint (a signup can precede any
// account), so an anonymous caller creates a tenant with no owner. When there
// *is* a user, they become its owner — otherwise nobody could administer the
// tenant they just made, since accepting an invitation would be the only way to
// ever gain a membership in it.
type Creator struct {
	UserID uuid.UUID
	Email  string
}

// Create provisions a new tenant and publishes tenant.created. When creator is
// non-nil, the creator is made an active owner member in the same transaction —
// tenant, owner membership and event all commit together or not at all.
func (s *Service) Create(ctx context.Context, slug, name string, settings map[string]any, creator *Creator) (ports.TenantInfo, error) {
	now := s.clk.Now().UTC()
	tenant, err := domain.New(s.ids.New(), slug, name, settings, now)
	if err != nil {
		return ports.TenantInfo{}, err
	}

	err = s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, tenant); err != nil {
			return err
		}
		if creator != nil {
			owner := &domain.Membership{
				ID:        s.ids.New(),
				TenantID:  tenant.ID,
				UserID:    creator.UserID,
				Email:     creator.Email,
				Role:      domain.RoleOwner,
				Status:    domain.MemberActive,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.members.CreateMembership(ctx, owner); err != nil {
				return err
			}
			// The owner joins like any other member, so consumers of
			// MemberJoined (seat counts, …) see them too — Accept() is not the
			// only way into a tenant any more.
			if _, err := s.outbox.Publish(ctx, events.EventInput{
				TenantID: tenant.ID,
				Module:   "tenant",
				Type:     ports.EventMemberJoined,
				Payload:  ports.MemberJoined{TenantID: tenant.ID, UserID: owner.UserID, Role: owner.Role},
			}); err != nil {
				return err
			}
		}
		_, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenant.ID,
			Module:   "tenant",
			Type:     ports.EventTenantCreated,
			Payload:  ports.TenantCreated{TenantID: tenant.ID, Slug: tenant.Slug, Name: tenant.Name},
		})
		return err
	})
	if err != nil {
		return ports.TenantInfo{}, err
	}
	return toInfo(tenant), nil
}

// Update changes a tenant's name and/or settings.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name string, settings map[string]any) (ports.TenantInfo, error) {
	var updated *domain.Tenant
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		t, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if name != "" {
			t.Name = name
		}
		if settings != nil {
			t.Settings = settings
		}
		t.UpdatedAt = s.clk.Now().UTC()
		if err := s.repo.Update(ctx, t); err != nil {
			return err
		}
		updated = t
		return nil
	})
	if err != nil {
		return ports.TenantInfo{}, err
	}
	return toInfo(updated), nil
}

// Suspend suspends an active tenant.
func (s *Service) Suspend(ctx context.Context, id uuid.UUID) error {
	return s.transition(ctx, id, (*domain.Tenant).Suspend, ports.EventTenantSuspended, ports.StatusSuspended)
}

// Reactivate reactivates a suspended tenant.
func (s *Service) Reactivate(ctx context.Context, id uuid.UUID) error {
	return s.transition(ctx, id, (*domain.Tenant).Reactivate, ports.EventTenantReactivated, ports.StatusActive)
}

// Delete soft-deletes a tenant.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.transition(ctx, id, (*domain.Tenant).Delete, ports.EventTenantDeleted, ports.StatusDeleted)
}

// transition loads a tenant, applies a guarded state change, persists it, and
// publishes the corresponding event — all in one transaction.
func (s *Service) transition(ctx context.Context, id uuid.UUID, apply func(*domain.Tenant) error, eventType string, newStatus ports.Status) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		t, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := apply(t); err != nil {
			return err
		}
		t.UpdatedAt = s.clk.Now().UTC()
		if err := s.repo.Update(ctx, t); err != nil {
			return err
		}
		_, err = s.outbox.Publish(ctx, events.EventInput{
			TenantID: t.ID,
			Module:   "tenant",
			Type:     eventType,
			Payload:  ports.TenantStatusChanged{TenantID: t.ID, Status: newStatus},
		})
		return err
	})
}

// GetByID implements ports.TenantReader.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (ports.TenantInfo, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ports.TenantInfo{}, err
	}
	return toInfo(t), nil
}

// GetBySlug implements ports.TenantReader.
func (s *Service) GetBySlug(ctx context.Context, slug string) (ports.TenantInfo, error) {
	normalized, err := domain.NormalizeSlug(slug)
	if err != nil {
		return ports.TenantInfo{}, err
	}
	t, err := s.repo.GetBySlug(ctx, normalized)
	if err != nil {
		return ports.TenantInfo{}, err
	}
	return toInfo(t), nil
}

func toInfo(t *domain.Tenant) ports.TenantInfo {
	return ports.TenantInfo{
		ID:     t.ID,
		Slug:   t.Slug,
		Name:   t.Name,
		Status: ports.Status(t.Status),
	}
}
