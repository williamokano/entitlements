/**
 * Typed client for the tenant-scoped entitlements read surface
 * (`/api/v1/entitlements`). Every route requires the bearer token AND the current
 * tenant (the shared api client attaches `X-Tenant-ID` in header mode) — the
 * effective set is resolved per tenant, so the active tenant selects which
 * entitlements are returned.
 *
 * Backend JSON shapes (see
 * internal/modules/entitlements/internal/adapters/rest/handler.go — getAll / getOne):
 *   GET /entitlements        -> { entitlements: { [key]: { value, source, expires_at? } } }
 *   GET /entitlements/{key}   -> { key, value, source, expires_at? }
 *
 * IMPORTANT backend-shape note: the read endpoints return only the effective
 * `value`, its winning `source` layer, and (for override-sourced values) an
 * optional `expires_at`. They do NOT return the feature's declared type
 * (boolean | limit | config | enum). The UI therefore infers how to render a
 * value from its JSON runtime type (see helpers.valueKind): boolean → on/off
 * chip, number → limit, string/other → config/enum value. This viewer is
 * read-only — overrides CRUD is F-011 and the usage panel is F-012.
 */

import { apiFetch } from '@/lib/api'

/** The layer a value won from, in precedence order default < plan < addon < override. */
export type Source = 'plan' | 'addon' | 'override' | 'default'

/** An effective entitlement value — the JSON runtime type mirrors the feature type. */
export type EntitlementValue = boolean | number | string | Record<string, unknown> | unknown[] | null

/** One feature's effective entitlement (the drill-in / per-key shape). */
export type Entitlement = {
  key: string
  value: EntitlementValue
  source: Source
  /** Present only for an override-sourced value that is time-bound. */
  expires_at?: string
}

/** A row in the effective-entitlements table: a key plus its resolved entry. */
export type EntitlementRow = Entitlement

const BASE = '/api/v1/entitlements'
const enc = encodeURIComponent

/** The raw per-key entry the summary endpoint nests under each feature key. */
type SummaryEntry = { value: EntitlementValue; source: Source; expires_at?: string }

/**
 * The tenant's whole effective entitlement set in a SINGLE call. The backend
 * returns a map keyed by feature key; we flatten it into rows sorted by key so
 * the table is stable and testable. An empty / no-subscription tenant still
 * returns its `default`-sourced rows.
 */
export async function listEntitlements(signal?: AbortSignal): Promise<EntitlementRow[]> {
  const res = await apiFetch<{ entitlements: Record<string, SummaryEntry> }>(BASE, { signal })
  const map = res.entitlements ?? {}
  return Object.keys(map)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => ({ key, value: map[key].value, source: map[key].source, expires_at: map[key].expires_at }))
}

/** One feature's effective entitlement (the per-feature drill-in). */
export async function getEntitlement(key: string, signal?: AbortSignal): Promise<Entitlement> {
  return apiFetch<Entitlement>(`${BASE}/${enc(key)}`, { signal })
}
