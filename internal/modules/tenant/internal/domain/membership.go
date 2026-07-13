package domain

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// MemberStatus is a membership's lifecycle state.
type MemberStatus string

// Membership states.
const (
	MemberActive  MemberStatus = "active"
	MemberRemoved MemberStatus = "removed"
)

// Membership binds a global user to a tenant with a role reference. Email is the
// address the member was invited by: memberships are only ever created by
// accepting an invitation, so the tenant keeps its own view of member identity
// rather than asking another module to resolve a user id. It is empty for
// memberships that predate the column.
type Membership struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	Email     string
	Role      string
	Status    MemberStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// InvitationStatus is an invitation's lifecycle state.
type InvitationStatus string

// Invitation states.
const (
	InvitePending  InvitationStatus = "pending"
	InviteAccepted InvitationStatus = "accepted"
	InviteDeclined InvitationStatus = "declined"
	InviteRevoked  InvitationStatus = "revoked"
)

// Invitation invites an email address (existing or future user) to join a tenant
// with a role. It is accepted later by the authenticated user whose email
// matches.
type Invitation struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Email     string
	Role      string
	Status    InvitationStatus
	InvitedBy uuid.UUID
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewInvitation builds a pending invitation with a normalized email.
func NewInvitation(id, tenantID, invitedBy uuid.UUID, email, role string, now, expiresAt time.Time) (*Invitation, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(role) == "" {
		return nil, apperr.Validation("invitation role must not be empty")
	}
	return &Invitation{
		ID:        id,
		TenantID:  tenantID,
		Email:     normalized,
		Role:      role,
		Status:    InvitePending,
		InvitedBy: invitedBy,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Acceptable reports whether the invitation can still be accepted at now:
// pending and unexpired.
func (i *Invitation) Acceptable(now time.Time) bool {
	return i.Status == InvitePending && !now.After(i.ExpiresAt)
}

// Pending reports whether the invitation is still open (pending and unexpired).
func (i *Invitation) Pending(now time.Time) bool { return i.Acceptable(now) }

const emailMaxLen = 254

// NormalizeEmail lowercases, trims, and validates an email address. It mirrors
// the authentication module's rule so an invited email matches the accepting
// user's normalized email.
func NormalizeEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", apperr.Validation("email must not be empty")
	}
	if len(s) > emailMaxLen {
		return "", apperr.Validation("email is too long")
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", apperr.Validation("email is not a valid address")
	}
	return s, nil
}

// MembershipRepository persists memberships.
type MembershipRepository interface {
	CreateMembership(ctx context.Context, m *Membership) error
	GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*Membership, error)
	ListMembers(ctx context.Context, tenantID uuid.UUID) ([]*Membership, error)
	RemoveMembership(ctx context.Context, tenantID, userID uuid.UUID, at time.Time) error
}

// InvitationRepository persists invitations.
type InvitationRepository interface {
	CreateInvitation(ctx context.Context, i *Invitation) error
	GetInvitation(ctx context.Context, tenantID, id uuid.UUID) (*Invitation, error)
	ListPendingInvitations(ctx context.Context, tenantID uuid.UUID) ([]*Invitation, error)
	UpdateInvitation(ctx context.Context, i *Invitation) error
}
