// Package rest is the authentication module's driving adapter: HTTP handlers for
// register, login, refresh, and logout, mounted under /api/v1/auth.
package rest

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/service"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// Authenticator resolves the identity behind a request's bearer access token.
// The module supplies one built from its JWT verifier; failure to resolve is an
// unauthenticated request.
type Authenticator func(r *http.Request) (ports.Identity, error)

// New returns the authentication module's HTTP handler. Routes are
// prefix-relative; the composition root mounts them under /api/v1/auth. authn
// authenticates the bearer token for the self-service (password/session) routes.
func New(svc *service.Service, authn Authenticator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", register(svc))
	mux.HandleFunc("POST /login", login(svc))
	mux.HandleFunc("POST /refresh", refresh(svc))
	mux.HandleFunc("POST /logout", logout(svc))

	// Email verification and password recovery (unauthenticated; token-driven).
	mux.HandleFunc("POST /verify-email/request", requestEmailVerification(svc))
	mux.HandleFunc("POST /verify-email", verifyEmail(svc))
	mux.HandleFunc("POST /password/forgot", requestPasswordReset(svc))
	mux.HandleFunc("POST /password/reset", resetPassword(svc))

	// Self-service (require a valid access token).
	mux.HandleFunc("POST /password/change", changePassword(svc, authn))
	mux.HandleFunc("GET /sessions", listSessions(svc, authn))
	mux.HandleFunc("POST /sessions/revoke-others", revokeOtherSessions(svc, authn))
	return mux
}

type emailBody struct {
	Email string `json:"email"`
}

type tokenBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func requestEmailVerification(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body emailBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if err := svc.RequestEmailVerification(r.Context(), body.Email); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		// Always 202, whether or not the email exists (no enumeration).
		w.WriteHeader(http.StatusAccepted)
	}
}

func verifyEmail(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body tokenBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if err := svc.VerifyEmail(r.Context(), body.Token); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func requestPasswordReset(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body emailBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if err := svc.RequestPasswordReset(r.Context(), body.Email); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func resetPassword(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body tokenBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if err := svc.ResetPassword(r.Context(), body.Token, body.Password); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func changePassword(svc *service.Service, authn Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ident, err := authn(r)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		var body struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if err := svc.ChangePassword(r.Context(), ident.UserID, ident.SessionID, body.CurrentPassword, body.NewPassword); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listSessions(svc *service.Service, authn Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ident, err := authn(r)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		sessions, err := svc.ListSessions(r.Context(), ident.UserID)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, map[string]any{
				"id":         s.ID,
				"issued_at":  s.IssuedAt,
				"expires_at": s.ExpiresAt,
				"current":    s.ID == ident.SessionID,
			})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sessions": out})
	}
}

func revokeOtherSessions(svc *service.Service, authn Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ident, err := authn(r)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		if err := svc.RevokeOtherSessions(r.Context(), ident.UserID, ident.SessionID); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// BearerToken extracts the token from an Authorization: Bearer <token> header.
func BearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

type credentialsBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}

func register(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body credentialsBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		view, err := svc.Register(r.Context(), body.Email, body.Password)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"id":     view.ID,
			"email":  view.Email,
			"status": view.Status,
		})
	}
}

func login(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body credentialsBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		pair, err := svc.Login(r.Context(), body.Email, body.Password)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		writeTokens(w, pair)
	}
}

func refresh(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body refreshBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if body.RefreshToken == "" {
			httpx.WriteProblem(w, r, apperr.Validation("refresh_token is required"))
			return
		}
		pair, err := svc.Refresh(r.Context(), body.RefreshToken)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		writeTokens(w, pair)
	}
}

func logout(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body refreshBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		if err := svc.Logout(r.Context(), body.RefreshToken); err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeTokens(w http.ResponseWriter, pair service.TokenPair) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":      pair.AccessToken,
		"refresh_token":     pair.RefreshToken,
		"token_type":        "Bearer",
		"access_expires_at": pair.AccessExpiresAt,
	})
}
