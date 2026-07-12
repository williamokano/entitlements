package authentication

import (
	"context"
	"net/http"
	"strings"

	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// NewAuthMiddleware authenticates a request from its Authorization header and
// populates the principal (and, for API keys, the tenant) in the context.
//
// It is permissive: a request with no credential passes through unauthenticated,
// so public routes (health, login, register) still work — a downstream guard
// (RequireAuth) rejects unauthenticated calls to protected routes. A credential
// that is present but invalid is rejected with 401 either way.
//
//   - "Authorization: Bearer <jwt>"  → Principal{Kind: user}.
//   - "Authorization: ApiKey <key>"  → Principal{Kind: machine, Scopes} plus the
//     key's tenant (so tenant resolution can be skipped for machine calls).
func NewAuthMiddleware(verifier ports.TokenVerifier, keys ports.APIKeyAuthenticator) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, credential, ok := parseAuthorization(r.Header.Get("Authorization"))
			if !ok {
				next.ServeHTTP(w, r) // no credential: pass through unauthenticated
				return
			}

			ctx := r.Context()
			switch strings.ToLower(scheme) {
			case "bearer":
				ident, err := verifier.Verify(credential)
				if err != nil {
					httpx.WriteProblem(w, r, apperr.Unauthorized("invalid access token"))
					return
				}
				ctx = authctx.WithPrincipal(ctx, authctx.Principal{
					Kind:    authctx.PrincipalUser,
					Subject: ident.UserID.String(),
					Email:   ident.Email,
				})
			case "apikey":
				mi, err := keys.AuthenticateAPIKey(ctx, credential)
				if err != nil {
					httpx.WriteProblem(w, r, err)
					return
				}
				ctx = authctx.WithPrincipal(ctx, authctx.Principal{
					Kind:    authctx.PrincipalMachine,
					Subject: mi.KeyID.String(),
					Scopes:  mi.Scopes,
				})
				// An API key is tenant-bound, so it carries its own tenant.
				ctx = authctx.WithTenantID(ctx, mi.TenantID)
			default:
				next.ServeHTTP(w, r) // unknown scheme: treat as unauthenticated
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthMiddleware returns the module's authentication middleware, wired to its
// JWT verifier and API-key authenticator.
func (m *Module) AuthMiddleware() httpx.Middleware {
	return NewAuthMiddleware(m.Verifier(), apiKeyAuthenticator{svc: m.svc})
}

// apiKeyAuthenticator adapts the service to ports.APIKeyAuthenticator.
type apiKeyAuthenticator struct{ svc apiKeyService }

type apiKeyService interface {
	AuthenticateAPIKey(ctx context.Context, rawKey string) (ports.MachineIdentity, error)
}

func (a apiKeyAuthenticator) AuthenticateAPIKey(ctx context.Context, rawKey string) (ports.MachineIdentity, error) {
	return a.svc.AuthenticateAPIKey(ctx, rawKey)
}

// RequireAuth rejects requests that carry no authenticated principal. Apply it
// to route groups that must not be reached anonymously.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authctx.PrincipalFromContext(r.Context()); !ok {
			httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseAuthorization splits an "Authorization: <scheme> <credential>" header.
func parseAuthorization(header string) (scheme, credential string, ok bool) {
	scheme, credential, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || scheme == "" || strings.TrimSpace(credential) == "" {
		return "", "", false
	}
	return scheme, strings.TrimSpace(credential), true
}
