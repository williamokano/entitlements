/**
 * Typed client for the tenant membership + invitation endpoints
 * (`/api/v1/tenants/{tenantId}/…`). The shared api client attaches the bearer
 * token and the tenant header; mutations that create something send an
 * Idempotency-Key per logical submit.
 *
 * Backend JSON shapes (see internal/modules/tenant/.../rest/membership.go):
 *   GET    /members                          -> { "members": [{ user_id, email, role, status }] }
 *   DELETE /members/{userId}                 -> 204 No Content
 *   GET    /invitations                      -> { "invitations": [{ id, email, role, status, expires_at, created_at }] }
 *   POST   /invitations                      -> the invitation (201)
 *   POST   /invitations/{invId}/resend       -> the invitation (200)
 *   POST   /invitations/{invId}/accept       -> { user_id, email, role, status } (201)
 *   POST   /invitations/{invId}/decline      -> 204 No Content
 *
 * `email` on a member is the address they were invited by. It is empty for
 * memberships written before the column existed — callers fall back to user_id.
 */

import { apiFetch, withIdempotencyKey } from '@/lib/api'

export type Member = {
  user_id: string
  email: string
  role: string
  status: string
}

export type Invitation = {
  id: string
  email: string
  role: string
  status: string
  expires_at: string
  created_at: string
}

/** The three roles the authorization module seeds into every new tenant. */
export const SYSTEM_ROLES = ['owner', 'admin', 'member'] as const

const tenantPath = (tenantId: string) => `/api/v1/tenants/${encodeURIComponent(tenantId)}`

export async function listMembers(tenantId: string, signal?: AbortSignal): Promise<Member[]> {
  const res = await apiFetch<{ members: Member[] | null }>(`${tenantPath(tenantId)}/members`, { signal })
  return res?.members ?? []
}

export async function removeMember(tenantId: string, userId: string): Promise<void> {
  await apiFetch<void>(`${tenantPath(tenantId)}/members/${encodeURIComponent(userId)}`, { method: 'DELETE' })
}

export async function listInvitations(tenantId: string, signal?: AbortSignal): Promise<Invitation[]> {
  const res = await apiFetch<{ invitations: Invitation[] | null }>(`${tenantPath(tenantId)}/invitations`, { signal })
  return res?.invitations ?? []
}

export async function createInvitation(tenantId: string, email: string, role: string): Promise<Invitation> {
  return apiFetch<Invitation>(`${tenantPath(tenantId)}/invitations`, {
    method: 'POST',
    body: { email, role },
    headers: withIdempotencyKey(),
  })
}

export async function resendInvitation(tenantId: string, invitationId: string): Promise<Invitation> {
  return apiFetch<Invitation>(`${tenantPath(tenantId)}/invitations/${encodeURIComponent(invitationId)}/resend`, {
    method: 'POST',
  })
}

export async function acceptInvitation(tenantId: string, invitationId: string): Promise<Member> {
  return apiFetch<Member>(`${tenantPath(tenantId)}/invitations/${encodeURIComponent(invitationId)}/accept`, {
    method: 'POST',
  })
}

export async function declineInvitation(tenantId: string, invitationId: string): Promise<void> {
  await apiFetch<void>(`${tenantPath(tenantId)}/invitations/${encodeURIComponent(invitationId)}/decline`, {
    method: 'POST',
  })
}

/**
 * Roles offered by the invite form. The invitation role is a free-form string,
 * so custom roles created via the authorization module are invitable too — but
 * a caller without `role:read` gets a 403 there, and the invite form must still
 * work, so any failure (or an empty tenant) falls back to the seeded system roles.
 */
export async function listInvitableRoles(signal?: AbortSignal): Promise<string[]> {
  try {
    const res = await apiFetch<{ roles: { name: string }[] | null }>('/api/v1/roles', { signal })
    const names = (res?.roles ?? []).map((role) => role.name).filter(Boolean)
    return names.length > 0 ? names : [...SYSTEM_ROLES]
  } catch {
    return [...SYSTEM_ROLES]
  }
}
