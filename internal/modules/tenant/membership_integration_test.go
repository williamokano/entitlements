//go:build integration

package tenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// serverAs wraps a handler so it sees an authenticated user principal, standing
// in for the global auth middleware.
func serverAs(h http.Handler, userID uuid.UUID, email string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{
			Kind:    authctx.PrincipalUser,
			Subject: userID.String(),
			Email:   email,
		})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}

// mkTenant creates a tenant through the admin API and returns its id.
func mkTenant(t *testing.T, srv *httptest.Server, slug string) uuid.UUID {
	t.Helper()
	status, body := post(t, srv.URL, `{"slug":"`+slug+`","name":"`+slug+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create tenant %s: status %d (%s)", slug, status, body)
	}
	return idOf(t, body)
}

// invite creates an invitation and returns its id.
func invite(t *testing.T, srv *httptest.Server, tenantID uuid.UUID, email, role string) uuid.UUID {
	t.Helper()
	status, body := post(t, srv.URL+"/"+tenantID.String()+"/invitations",
		`{"email":"`+email+`","role":"`+role+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("invite %s: status %d (%s)", email, status, body)
	}
	return idOf(t, body)
}

func TestInviteNewEmailThenRegisterThenAcceptBecomesMember(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps)
	inviter := serverAs(mod.Handler(), uuid.New(), "owner@example.com")
	defer inviter.Close()

	tenantID := mkTenant(t, inviter, "acme")
	invID := invite(t, inviter, tenantID, "newbie@example.com", "member")

	// The invited person registers later (a fresh user id) and accepts.
	newUserID := uuid.New()
	invitee := serverAs(mod.Handler(), newUserID, "newbie@example.com")
	defer invitee.Close()

	status, body := post(t, invitee.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/accept", "")
	if status != http.StatusCreated {
		t.Fatalf("accept: status %d (%s), want 201", status, body)
	}

	// The reader now returns the membership with its role.
	info, err := mod.MembershipPort().GetMembership(context.Background(), tenantID, newUserID)
	if err != nil || info.Role != "member" || !info.Active {
		t.Fatalf("GetMembership = (%+v, %v), want active member", info, err)
	}
}

func TestInviteExistingUserAcceptFlowEmitsMemberJoined(t *testing.T) {
	deps, bus := newDeps(t)
	mod := tenant.New(deps)
	var joined int
	var mu sync.Mutex
	bus.Subscribe(ports.EventMemberJoined, func(_ context.Context, _ events.Event) error {
		mu.Lock()
		joined++
		mu.Unlock()
		return nil
	})
	relay := events.NewRelay(deps.Pool, bus, deps.Clock, deps.Logger, events.RelayConfig{})

	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()
	tenantID := mkTenant(t, admin, "acme")
	invID := invite(t, admin, tenantID, "member@example.com", "admin")

	userID := uuid.New()
	member := serverAs(mod.Handler(), userID, "member@example.com")
	defer member.Close()
	if status, _ := post(t, member.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/accept", ""); status != http.StatusCreated {
		t.Fatal("accept, want 201")
	}

	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("relay: %v", err)
	}
	mu.Lock()
	got := joined
	mu.Unlock()
	if got < 1 {
		t.Fatalf("member.joined events = %d, want >= 1", got)
	}
}

func TestInvitationExpiryRejected(t *testing.T) {
	frozen := clock.NewFrozen(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	deps, _ := newDepsClock(t, frozen)
	mod := tenant.New(deps)
	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()

	tenantID := mkTenant(t, admin, "acme")
	invID := invite(t, admin, tenantID, "late@example.com", "member")

	// Advance well past the invitation TTL (7 days).
	frozen.Advance(8 * 24 * time.Hour)

	invitee := serverAs(mod.Handler(), uuid.New(), "late@example.com")
	defer invitee.Close()
	if status, _ := post(t, invitee.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/accept", ""); status != http.StatusConflict {
		t.Fatalf("expired accept status, want 409")
	}
}

func TestResendExtendsExpiry(t *testing.T) {
	frozen := clock.NewFrozen(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	deps, _ := newDepsClock(t, frozen)
	mod := tenant.New(deps)
	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()

	tenantID := mkTenant(t, admin, "acme")
	invID := invite(t, admin, tenantID, "resend@example.com", "member")

	// Just before the original expiry, resend to extend it, then advance past
	// the original expiry — acceptance still works.
	frozen.Advance(6 * 24 * time.Hour)
	if status, _ := post(t, admin.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/resend", ""); status != http.StatusOK {
		t.Fatal("resend, want 200")
	}
	frozen.Advance(3 * 24 * time.Hour) // past the original 7-day expiry, within the new one

	invitee := serverAs(mod.Handler(), uuid.New(), "resend@example.com")
	defer invitee.Close()
	if status, _ := post(t, invitee.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/accept", ""); status != http.StatusCreated {
		t.Fatal("accept after resend, want 201")
	}
}

func TestDeclineInvitationLeavesNoMembership(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps)
	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()

	tenantID := mkTenant(t, admin, "acme")
	invID := invite(t, admin, tenantID, "declines@example.com", "member")

	userID := uuid.New()
	invitee := serverAs(mod.Handler(), userID, "declines@example.com")
	defer invitee.Close()
	if status, _ := post(t, invitee.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/decline", ""); status != http.StatusNoContent {
		t.Fatal("decline, want 204")
	}
	// No membership; a subsequent accept fails (not pending).
	if _, err := mod.MembershipPort().GetMembership(context.Background(), tenantID, userID); err == nil {
		t.Fatal("membership exists after decline")
	}
	if status, _ := post(t, invitee.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/accept", ""); status != http.StatusConflict {
		t.Fatal("accept after decline, want 409")
	}
}

func TestDuplicatePendingInvitationConflict(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps)
	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()

	tenantID := mkTenant(t, admin, "acme")
	invite(t, admin, tenantID, "dup@example.com", "member")
	if status, _ := post(t, admin.URL+"/"+tenantID.String()+"/invitations", `{"email":"dup@example.com","role":"member"}`); status != http.StatusConflict {
		t.Fatal("duplicate pending invite, want 409")
	}
}

func TestUserInMultipleTenantsIndependentMemberships(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps)
	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()

	tenantA := mkTenant(t, admin, "aco")
	tenantB := mkTenant(t, admin, "bco")
	invA := invite(t, admin, tenantA, "multi@example.com", "owner")
	invB := invite(t, admin, tenantB, "multi@example.com", "member")

	userID := uuid.New()
	user := serverAs(mod.Handler(), userID, "multi@example.com")
	defer user.Close()
	if status, _ := post(t, user.URL+"/"+tenantA.String()+"/invitations/"+invA.String()+"/accept", ""); status != http.StatusCreated {
		t.Fatal("accept A, want 201")
	}
	if status, _ := post(t, user.URL+"/"+tenantB.String()+"/invitations/"+invB.String()+"/accept", ""); status != http.StatusCreated {
		t.Fatal("accept B, want 201")
	}

	a, _ := mod.MembershipPort().GetMembership(context.Background(), tenantA, userID)
	b, _ := mod.MembershipPort().GetMembership(context.Background(), tenantB, userID)
	if a.Role != "owner" || b.Role != "member" {
		t.Fatalf("independent roles = (%s, %s), want (owner, member)", a.Role, b.Role)
	}
}

func TestRemoveMemberEmitsMemberLeftAndRevokesAccess(t *testing.T) {
	deps, bus := newDeps(t)
	mod := tenant.New(deps)
	var left int
	var mu sync.Mutex
	bus.Subscribe(ports.EventMemberLeft, func(_ context.Context, _ events.Event) error {
		mu.Lock()
		left++
		mu.Unlock()
		return nil
	})
	relay := events.NewRelay(deps.Pool, bus, deps.Clock, deps.Logger, events.RelayConfig{})

	admin := serverAs(mod.Handler(), uuid.New(), "admin@example.com")
	defer admin.Close()
	tenantID := mkTenant(t, admin, "acme")
	invID := invite(t, admin, tenantID, "leaver@example.com", "member")

	userID := uuid.New()
	user := serverAs(mod.Handler(), userID, "leaver@example.com")
	defer user.Close()
	if status, _ := post(t, user.URL+"/"+tenantID.String()+"/invitations/"+invID.String()+"/accept", ""); status != http.StatusCreated {
		t.Fatal("accept, want 201")
	}

	// Remove the member.
	if status, _ := do(t, http.MethodDelete, admin.URL+"/"+tenantID.String()+"/members/"+userID.String(), ""); status != http.StatusNoContent {
		t.Fatal("remove member, want 204")
	}
	// Access revoked: reader no longer returns the membership.
	if _, err := mod.MembershipPort().GetMembership(context.Background(), tenantID, userID); err == nil {
		t.Fatal("membership still active after removal")
	}
	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("relay: %v", err)
	}
	mu.Lock()
	got := left
	mu.Unlock()
	if got < 1 {
		t.Fatalf("member.left events = %d, want >= 1", got)
	}
}

func TestUnauthenticatedMembershipRouteRejected(t *testing.T) {
	deps, _ := newDeps(t)
	mod := tenant.New(deps)
	// No principal injected.
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	tid := uuid.New()
	if status, _ := post(t, srv.URL+"/"+tid.String()+"/invitations", `{"email":"x@example.com","role":"member"}`); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated invite status, want 401")
	}
}
