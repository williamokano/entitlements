// Package rest is the authentication module's driving adapter: HTTP handlers for
// register, login, refresh, and logout, mounted under /api/v1/auth.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the authentication module's HTTP handler. Routes are
// prefix-relative; the composition root mounts them under /api/v1/auth.
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", register(svc))
	mux.HandleFunc("POST /login", login(svc))
	mux.HandleFunc("POST /refresh", refresh(svc))
	mux.HandleFunc("POST /logout", logout(svc))
	return mux
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
