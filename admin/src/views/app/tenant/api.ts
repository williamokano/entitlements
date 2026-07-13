/**
 * Typed client for the tenant endpoints (`/api/v1/tenants`). Auth is global but
 * these routes require a bearer token; the shared api client attaches it.
 *
 * Backend JSON shapes (see internal/modules/tenant/.../rest/handler.go):
 *   POST   /tenants                    { slug, name, settings? } -> { id, slug, name, status }
 *   GET    /tenants/{id}               -> { id, slug, name, status }
 *   PATCH  /tenants/{id}               { name?, settings? } -> { id, slug, name, status }
 *   POST   /tenants/{id}/suspend       -> 204
 *   POST   /tenants/{id}/reactivate    -> 204
 *   DELETE /tenants/{id}               -> 204
 *
 * NOTE: the backend response does not currently echo `settings`, and PATCH does
 * not accept a slug change — the slug is fixed at creation. The UI reflects
 * that (slug is read-only on the settings screen).
 */

import { apiFetch, withIdempotencyKey } from '@/lib/api'

export type TenantStatus = string

export type Tenant = {
  id: string
  slug: string
  name: string
  status: TenantStatus
  settings?: Record<string, unknown>
}

const PATH = '/api/v1/tenants'

export async function createTenant(input: { name: string; slug: string; settings?: Record<string, unknown> }): Promise<Tenant> {
  return apiFetch<Tenant>(PATH, {
    method: 'POST',
    body: { name: input.name, slug: input.slug, settings: input.settings ?? {} },
    headers: withIdempotencyKey(),
    // Creation happens during onboarding before a tenant is selected; the
    // request carries the bearer token but must not require a tenant header.
  })
}

export async function getTenant(id: string, signal?: AbortSignal): Promise<Tenant> {
  return apiFetch<Tenant>(`${PATH}/${encodeURIComponent(id)}`, { signal })
}

export async function updateTenant(id: string, input: { name?: string; settings?: Record<string, unknown> }): Promise<Tenant> {
  return apiFetch<Tenant>(`${PATH}/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: input,
  })
}

export async function suspendTenant(id: string): Promise<void> {
  await apiFetch<void>(`${PATH}/${encodeURIComponent(id)}/suspend`, { method: 'POST' })
}

export async function reactivateTenant(id: string): Promise<void> {
  await apiFetch<void>(`${PATH}/${encodeURIComponent(id)}/reactivate`, { method: 'POST' })
}

export async function deleteTenant(id: string): Promise<void> {
  await apiFetch<void>(`${PATH}/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
