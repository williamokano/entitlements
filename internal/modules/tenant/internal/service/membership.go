package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// MembershipService holds the membership and invitation use cases. Each mutation
// and its published event share one transaction (via the outbox).
type MembershipService struct {
	uow       *postgres.UnitOfWork
	outbox    *events.Outbox
	members   domain.MembershipRepository
	invites   domain.InvitationRepository
	ids       id.Generator
	clk       clock.Clock
	inviteTTL time.Duration
}

// NewMembershipService builds a MembershipService.
func NewMembershipService(uow *postgres.UnitOfWork, outbox *events.Outbox, members domain.MembershipRepository, invites domain.InvitationRepository, ids id.Generator, clk clock.Clock, inviteTTL time.Duration) *MembershipService {
	return &MembershipService{uow: uow, outbox: outbox, members: members, invites: invites, ids: ids, clk: clk, inviteTTL: inviteTTL}
}

// InvitationView is a read model of an invitation.
type InvitationView struct {
	ID        uuid.UUID
	Email     string
	Role      string
	Status    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// MembershipView is a read model of a membership.
type MembershipView struct {
	UserID uuid.UUID
	Role   string
	Status string
}

// Invite creates a pending invitation for an email to join the tenant and
// publishes tenant.invitation.sent. A second pending invite for the same email
// conflicts (409).
func (s *MembershipService) Invite(ctx context.Context, tenantID, invitedBy uuid.UUID, email, role string) (InvitationView, error) {
	now := s.clk.Now().UTC()
	inv, err := domain.NewInvitation(s.ids.New(), tenantID, invitedBy, email, role, now, now.Add(s.inviteTTL))
	if err != nil {
		return InvitationView{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.invites.CreateInvitation(ctx, inv); err != nil {
			return err
		}
		return s.publishInvitationSent(ctx, inv)
	}); err != nil {
		return InvitationView{}, err
	}
	return toInvitationView(inv), nil
}

// Resend extends a pending invitation's expiry and re-publishes the event.
func (s *MembershipService) Resend(ctx context.Context, tenantID, invitationID uuid.UUID) (InvitationView, error) {
	var view InvitationView
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		inv, err := s.invites.GetInvitation(ctx, tenantID, invitationID)
		if err != nil {
			return err
		}
		if inv.Status != domain.InvitePending {
			return apperr.Conflict("invitation is not pending")
		}
		now := s.clk.Now().UTC()
		inv.ExpiresAt = now.Add(s.inviteTTL)
		inv.UpdatedAt = now
		if err := s.invites.UpdateInvitation(ctx, inv); err != nil {
			return err
		}
		if err := s.publishInvitationSent(ctx, inv); err != nil {
			return err
		}
		view = toInvitationView(inv)
		return nil
	})
	return view, err
}

// Accept turns an invitation into a membership for the authenticated user whose
// email matches the invitation, and publishes tenant.member.joined.
func (s *MembershipService) Accept(ctx context.Context, tenantID, invitationID, userID uuid.UUID, userEmail string) (MembershipView, error) {
	normalized, err := domain.NormalizeEmail(userEmail)
	if err != nil {
		return MembershipView{}, apperr.Forbidden("invitation is not for this user")
	}
	var view MembershipView
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		inv, err := s.invites.GetInvitation(ctx, tenantID, invitationID)
		if err != nil {
			return err
		}
		if !inv.Acceptable(s.clk.Now().UTC()) {
			return apperr.Conflict("invitation is no longer acceptable")
		}
		if inv.Email != normalized {
			return apperr.Forbidden("invitation is not for this user")
		}
		now := s.clk.Now().UTC()
		m := &domain.Membership{
			ID:        s.ids.New(),
			TenantID:  tenantID,
			UserID:    userID,
			Role:      inv.Role,
			Status:    domain.MemberActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.members.CreateMembership(ctx, m); err != nil {
			return err
		}
		inv.Status = domain.InviteAccepted
		inv.UpdatedAt = now
		if err := s.invites.UpdateInvitation(ctx, inv); err != nil {
			return err
		}
		if _, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenantID,
			Module:   "tenant",
			Type:     ports.EventMemberJoined,
			Payload:  ports.MemberJoined{TenantID: tenantID, UserID: userID, Role: inv.Role},
		}); err != nil {
			return err
		}
		view = MembershipView{UserID: userID, Role: inv.Role, Status: string(domain.MemberActive)}
		return nil
	})
	return view, err
}

// Decline marks an invitation declined for the authenticated matching user. No
// membership is created.
func (s *MembershipService) Decline(ctx context.Context, tenantID, invitationID uuid.UUID, userEmail string) error {
	normalized, err := domain.NormalizeEmail(userEmail)
	if err != nil {
		return apperr.Forbidden("invitation is not for this user")
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		inv, err := s.invites.GetInvitation(ctx, tenantID, invitationID)
		if err != nil {
			return err
		}
		if inv.Status != domain.InvitePending {
			return apperr.Conflict("invitation is not pending")
		}
		if inv.Email != normalized {
			return apperr.Forbidden("invitation is not for this user")
		}
		inv.Status = domain.InviteDeclined
		inv.UpdatedAt = s.clk.Now().UTC()
		return s.invites.UpdateInvitation(ctx, inv)
	})
}

// ListInvitations returns the tenant's pending invitations.
func (s *MembershipService) ListInvitations(ctx context.Context, tenantID uuid.UUID) ([]InvitationView, error) {
	invs, err := s.invites.ListPendingInvitations(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	views := make([]InvitationView, 0, len(invs))
	for _, i := range invs {
		views = append(views, toInvitationView(i))
	}
	return views, nil
}

// ListMembers returns the tenant's active members.
func (s *MembershipService) ListMembers(ctx context.Context, tenantID uuid.UUID) ([]MembershipView, error) {
	members, err := s.members.ListMembers(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	views := make([]MembershipView, 0, len(members))
	for _, m := range members {
		views = append(views, MembershipView{UserID: m.UserID, Role: m.Role, Status: string(m.Status)})
	}
	return views, nil
}

// RemoveMember removes a user from the tenant and publishes tenant.member.left.
func (s *MembershipService) RemoveMember(ctx context.Context, tenantID, userID uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.members.RemoveMembership(ctx, tenantID, userID, s.clk.Now().UTC()); err != nil {
			return err
		}
		_, err := s.outbox.Publish(ctx, events.EventInput{
			TenantID: tenantID,
			Module:   "tenant",
			Type:     ports.EventMemberLeft,
			Payload:  ports.MemberLeft{TenantID: tenantID, UserID: userID},
		})
		return err
	})
}

// GetMembership implements ports.MembershipReader.
func (s *MembershipService) GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (ports.MembershipInfo, error) {
	m, err := s.members.GetMembership(ctx, tenantID, userID)
	if err != nil {
		return ports.MembershipInfo{}, err
	}
	return ports.MembershipInfo{
		TenantID: m.TenantID,
		UserID:   m.UserID,
		Role:     m.Role,
		Active:   m.Status == domain.MemberActive,
	}, nil
}

func (s *MembershipService) publishInvitationSent(ctx context.Context, inv *domain.Invitation) error {
	_, err := s.outbox.Publish(ctx, events.EventInput{
		TenantID: inv.TenantID,
		Module:   "tenant",
		Type:     ports.EventInvitationSent,
		Payload:  ports.InvitationSent{TenantID: inv.TenantID, InvitationID: inv.ID, Email: inv.Email, Role: inv.Role},
	})
	return err
}

func toInvitationView(i *domain.Invitation) InvitationView {
	return InvitationView{
		ID:        i.ID,
		Email:     i.Email,
		Role:      i.Role,
		Status:    string(i.Status),
		ExpiresAt: i.ExpiresAt,
		CreatedAt: i.CreatedAt,
	}
}
