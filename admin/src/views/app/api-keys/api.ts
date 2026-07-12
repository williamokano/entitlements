/**
 * Typed client for the tenant-scoped API-key endpoints
 * (`/api/v1/api-keys`). The shared api client attaches the bearer token and
 * the tenant header; create sends an Idempotency-Key per logical submit.
 *
 * Backend JSON shapes (see internal/.../rest/apikeys.go):
 *   GET    -> { "api_keys": [{ id, prefix, name, scopes, created_at, last_used_at }] }
 *   POST   -> { id, prefix, name, scopes, api_key, created_at }   // api_key shown ONCE
 *   DELETE -> 204 No Content
 */

import { apiFetch, withIdempotencyKey } from '@/lib/api'

export type ApiKey = {
  id: string
  prefix: string
  name: string
  scopes: string[]
  created_at: string
  last_used_at: string | null
}

/** The create response additionally carries the one-time full secret. */
export type CreatedApiKey = {
  id: string
  prefix: string
  name: string
  scopes: string[]
  /** Full secret ("ak_....<secret>") — returned exactly once, never again. */
  api_key: string
  created_at: string
}

const PATH = '/api/v1/api-keys'

export async function listApiKeys(signal?: AbortSignal): Promise<ApiKey[]> {
  const res = await apiFetch<{ api_keys: ApiKey[] | null }>(PATH, { signal })
  return res?.api_keys ?? []
}

export async function createApiKey(name: string, scopes: string[]): Promise<CreatedApiKey> {
  return apiFetch<CreatedApiKey>(PATH, {
    method: 'POST',
    body: { name, scopes },
    headers: withIdempotencyKey(),
  })
}

export async function revokeApiKey(id: string): Promise<void> {
  await apiFetch<void>(`${PATH}/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
