// Package authctx carries the authenticated Principal (user or API key) and
// the resolved TenantID through the request context.
//
// It exposes helpers such as MustTenant for repositories that refuse to run
// without a tenant, plus a WithSystemContext escape hatch for admin/job
// paths. Populated by middleware implemented in T-011 and T-014.
package authctx
