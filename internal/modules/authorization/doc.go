// Package authorization is dynamic, data-driven RBAC: tenant-scoped roles
// carrying permission strings (resource:action), seeded system roles, and
// role assignment to members.
//
// It exposes an Authorizer port (Check / RequirePermission middleware)
// designed so a policy engine (ABAC/Cedar/OPA) can replace the RBAC
// implementation later. Hexagonal layout. Implemented in T-016.
package authorization
