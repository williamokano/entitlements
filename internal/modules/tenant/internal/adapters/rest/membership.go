package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// registerMembership wires the membership and invitation routes onto the tenant
// module's mux (mounted under /api/v1/tenants). These routes require an
// authenticated user; accept/decline additionally match the caller's email.
func registerMembership(mux *http.ServeMux, svc *service.MembershipService) {
	mux.HandleFunc("POST /{id}/invitations", createInvitation(svc))
	mux.HandleFunc("GET /{id}/invitations", listInvitations(svc))
	mux.HandleFunc("POST /{id}/invitations/{invId}/resend", resendInvitation(svc))
	mux.HandleFunc("POST /{id}/invitations/{invId}/accept", acceptInvitation(svc))
	mux.HandleFunc("POST /{id}/invitations/{invId}/decline", declineInvitation(svc))
	mux.HandleFunc("GET /{id}/members", listMembers(svc))
	mux.HandleFunc("DELETE /{id}/members/{userId}", removeMember(svc))
}

func createInvitation(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireUser(w, r)
		if !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		var body struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.Invite(r.Context(), tenantID, p.subject, body.Email, body.Role)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, invitationResponse(view))
	}
}

func listInvitations(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireUser(w, r); !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		invs, err := svc.ListInvitations(r.Context(), tenantID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(invs))
		for _, i := range invs {
			out = append(out, invitationResponse(i))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"invitations": out})
	}
}

func resendInvitation(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireUser(w, r); !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		invID, ok := parsePathID(w, r, "invId", "invitation id")
		if !ok {
			return
		}
		view, err := svc.Resend(r.Context(), tenantID, invID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invitationResponse(view))
	}
}

func acceptInvitation(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireUser(w, r)
		if !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		invID, ok := parsePathID(w, r, "invId", "invitation id")
		if !ok {
			return
		}
		view, err := svc.Accept(r.Context(), tenantID, invID, p.subject, p.email)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, membershipResponse(view))
	}
}

func declineInvitation(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireUser(w, r)
		if !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		invID, ok := parsePathID(w, r, "invId", "invitation id")
		if !ok {
			return
		}
		if err := svc.Decline(r.Context(), tenantID, invID, p.email); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listMembers(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireUser(w, r); !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		members, err := svc.ListMembers(r.Context(), tenantID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(members))
		for _, m := range members {
			out = append(out, membershipResponse(m))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"members": out})
	}
}

func removeMember(svc *service.MembershipService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireUser(w, r); !ok {
			return
		}
		tenantID, ok := parsePathID(w, r, "id", "tenant id")
		if !ok {
			return
		}
		userID, ok := parsePathID(w, r, "userId", "user id")
		if !ok {
			return
		}
		if err := svc.RemoveMember(r.Context(), tenantID, userID); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// currentUser is the authenticated user extracted from the request principal.
type currentUser struct {
	subject uuid.UUID
	email   string
}

// requireUser resolves the authenticated user principal, writing a 401 and
// returning ok=false when the caller is not an authenticated user.
func requireUser(w http.ResponseWriter, r *http.Request) (currentUser, bool) {
	p, ok := authctx.PrincipalFromContext(r.Context())
	if !ok || p.Kind != authctx.PrincipalUser {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return currentUser{}, false
	}
	id, err := uuid.Parse(p.Subject)
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return currentUser{}, false
	}
	return currentUser{subject: id, email: p.Email}, true
}

func parsePathID(w http.ResponseWriter, r *http.Request, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid "+label))
		return uuid.Nil, false
	}
	return id, true
}

// membershipResponse renders a membership. Email is the address the member was
// invited by; it is empty for memberships that predate the column, and clients
// fall back to the user id.
func membershipResponse(m service.MembershipView) map[string]any {
	return map[string]any{
		"user_id": m.UserID,
		"email":   m.Email,
		"role":    m.Role,
		"status":  m.Status,
	}
}

func invitationResponse(i service.InvitationView) map[string]any {
	return map[string]any{
		"id":         i.ID,
		"email":      i.Email,
		"role":       i.Role,
		"status":     i.Status,
		"expires_at": i.ExpiresAt,
		"created_at": i.CreatedAt,
	}
}
