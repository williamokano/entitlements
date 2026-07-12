// Package authorization is the RBAC module: tenant-scoped roles (data, not code),
// role assignment to users, and a permission check enforced at the HTTP layer.
// The implementation is replaceable behind ports.Authorizer.
package authorization

import (
	"net/http"

	"github.com/williamokano/entitlements/internal/modules/authorization/ports"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// RequirePermission returns middleware that rejects a request unless the caller
// holds the given permission. A denial is an RFC 7807 problem+json 403 (401 when
// unauthenticated), produced by the authorizer's error.
func RequirePermission(authz ports.Authorizer, permission string) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := authz.Check(r.Context(), permission); err != nil {
				httpx.WriteProblem(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
