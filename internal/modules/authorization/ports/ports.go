// Package ports is the authorization module's public surface: the Authorizer
// other modules and the HTTP middleware depend on. The RBAC implementation is
// replaceable behind this interface (e.g. by a policy engine) without touching
// callers.
package ports

import "context"

// Authorizer decides whether the caller in ctx holds a permission. It reads the
// principal and tenant from the context (populated by the auth and tenant
// middleware) and returns nil when allowed or a forbidden error otherwise.
type Authorizer interface {
	Check(ctx context.Context, permission string) error
}
