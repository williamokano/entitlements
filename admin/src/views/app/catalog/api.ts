/**
 * Typed client for the catalog admin surface (`/api/v1/catalog`). The catalog is
 * operator-global (not tenant-scoped) but every admin route still requires the
 * bearer token — the shared api client attaches it. The public listing is open.
 *
 * Backend JSON shapes (see internal/modules/catalog/internal/adapters/rest/*.go):
 *   Plans:   POST /plans {key,name} -> { plan, version }
 *            GET  /plans            -> { plans: [plan] }
 *            GET  /plans/{id}       -> { plan, versions: [version] }
 *            POST /plans/{id}/public {public} -> plan
 *            POST /plans/{id}/archive         -> 204
 *            POST /plans/{id}/versions        -> version (new draft)
 *            PATCH /plans/{id}/versions/{vid} -> version (draft only)
 *            POST  /plans/{id}/versions/{vid}/publish -> version
 *            GET   /versions/{vid}  -> versionDetail (+ plan_key)
 *            GET   /public          -> { plans: [{ plan, version }] }
 *   Addons:  POST /addons {key,name} -> { addon, version }
 *            GET  /addons            -> { addons: [addon] }
 *            GET  /addons/{id}       -> { addon, versions: [addonVersion] }
 *            POST /addons/{id}/archive          -> 204
 *            POST /addons/{id}/versions         -> addonVersion (new draft)
 *            PATCH /addons/{id}/versions/{vid}  -> addonVersion (draft only)
 *            POST  /addons/{id}/versions/{vid}/publish -> addonVersion
 *            GET   /addon-versions/{vid}        -> addonVersionDetail (+ addon_key)
 *
 * Money is ALWAYS integer minor units + a 3-letter currency code — never floats.
 */

import { apiFetch, withIdempotencyKey } from '@/lib/api'

export type PlanStatus = 'draft' | 'active' | 'archived'
export type VersionStatus = 'draft' | 'published'
export type BillingCycle = 'monthly' | 'annual' | 'custom'
export type DeltaKind = 'limit_delta' | 'value_override'

export const BILLING_CYCLES: BillingCycle[] = ['monthly', 'annual', 'custom']

/** An amount charged per billing cycle, in integer minor units (never a float). */
export type Price = {
  cycle: BillingCycle
  amount_minor: number
}

export type TrialConfig = {
  enabled: boolean
  days: number
  card_required: boolean
}

export type Plan = {
  id: string
  key: string
  name: string
  status: PlanStatus
  public: boolean
}

export type PlanVersion = {
  id: string
  plan_id: string
  version: number
  status: VersionStatus
  currency: string
  prices: Price[]
  trial: TrialConfig
  grace_days: number
  feature_grants: Record<string, unknown>
  published_at: string | null
}

/** GET /versions/{vid} additionally carries the plan key (for the breadcrumb). */
export type PlanVersionDetail = PlanVersion & { plan_key: string }

export type Addon = {
  id: string
  key: string
  name: string
  status: PlanStatus
}

/** An entitlement change an addon applies to a feature key. */
export type Delta = {
  feature_key: string
  kind: DeltaKind
  amount?: number
  value?: unknown
}

export type AddonVersion = {
  id: string
  addon_id: string
  version: number
  status: VersionStatus
  currency: string
  prices: Price[]
  quantity_allowed: boolean
  compatible_plan_keys: string[]
  deltas: Delta[]
}

export type AddonVersionDetail = AddonVersion & { addon_key: string }

export type PublicPlan = { plan: Plan; version: PlanVersion }

export type VersionContent = {
  currency: string
  prices: Price[]
  trial: TrialConfig
  grace_days: number
  feature_grants: Record<string, unknown>
}

export type AddonVersionContent = {
  currency: string
  prices: Price[]
  quantity_allowed: boolean
  compatible_plan_keys: string[]
  deltas: Delta[]
}

const BASE = '/api/v1/catalog'
const enc = encodeURIComponent

// --- Plans -----------------------------------------------------------------

export async function listPlans(signal?: AbortSignal): Promise<Plan[]> {
  const res = await apiFetch<{ plans: Plan[] | null }>(`${BASE}/plans`, { signal })
  return res?.plans ?? []
}

export async function createPlan(key: string, name: string): Promise<{ plan: Plan; version: PlanVersion }> {
  return apiFetch(`${BASE}/plans`, { method: 'POST', body: { key, name }, headers: withIdempotencyKey() })
}

export async function getPlan(id: string, signal?: AbortSignal): Promise<{ plan: Plan; versions: PlanVersion[] }> {
  const res = await apiFetch<{ plan: Plan; versions: PlanVersion[] | null }>(`${BASE}/plans/${enc(id)}`, { signal })
  return { plan: res.plan, versions: res.versions ?? [] }
}

export async function setPlanPublic(id: string, isPublic: boolean): Promise<Plan> {
  return apiFetch(`${BASE}/plans/${enc(id)}/public`, { method: 'POST', body: { public: isPublic } })
}

export async function archivePlan(id: string): Promise<void> {
  await apiFetch<void>(`${BASE}/plans/${enc(id)}/archive`, { method: 'POST' })
}

export async function createPlanVersion(id: string): Promise<PlanVersion> {
  return apiFetch(`${BASE}/plans/${enc(id)}/versions`, { method: 'POST', headers: withIdempotencyKey() })
}

export async function getPlanVersion(vid: string, signal?: AbortSignal): Promise<PlanVersionDetail> {
  return apiFetch(`${BASE}/versions/${enc(vid)}`, { signal })
}

export async function updatePlanVersion(planId: string, vid: string, content: VersionContent): Promise<PlanVersion> {
  return apiFetch(`${BASE}/plans/${enc(planId)}/versions/${enc(vid)}`, { method: 'PATCH', body: content })
}

export async function publishPlanVersion(planId: string, vid: string): Promise<PlanVersion> {
  return apiFetch(`${BASE}/plans/${enc(planId)}/versions/${enc(vid)}/publish`, {
    method: 'POST',
    headers: withIdempotencyKey(),
  })
}

export async function listPublicPlans(signal?: AbortSignal): Promise<PublicPlan[]> {
  // Public listing — no bearer token required.
  const res = await apiFetch<{ plans: PublicPlan[] | null }>(`${BASE}/public`, { signal, auth: false })
  return res?.plans ?? []
}

// --- Addons ----------------------------------------------------------------

export async function listAddons(signal?: AbortSignal): Promise<Addon[]> {
  const res = await apiFetch<{ addons: Addon[] | null }>(`${BASE}/addons`, { signal })
  return res?.addons ?? []
}

export async function createAddon(key: string, name: string): Promise<{ addon: Addon; version: AddonVersion }> {
  return apiFetch(`${BASE}/addons`, { method: 'POST', body: { key, name }, headers: withIdempotencyKey() })
}

export async function getAddon(id: string, signal?: AbortSignal): Promise<{ addon: Addon; versions: AddonVersion[] }> {
  const res = await apiFetch<{ addon: Addon; versions: AddonVersion[] | null }>(`${BASE}/addons/${enc(id)}`, { signal })
  return { addon: res.addon, versions: res.versions ?? [] }
}

export async function archiveAddon(id: string): Promise<void> {
  await apiFetch<void>(`${BASE}/addons/${enc(id)}/archive`, { method: 'POST' })
}

export async function createAddonVersion(id: string): Promise<AddonVersion> {
  return apiFetch(`${BASE}/addons/${enc(id)}/versions`, { method: 'POST', headers: withIdempotencyKey() })
}

export async function getAddonVersion(vid: string, signal?: AbortSignal): Promise<AddonVersionDetail> {
  return apiFetch(`${BASE}/addon-versions/${enc(vid)}`, { signal })
}

export async function updateAddonVersion(addonId: string, vid: string, content: AddonVersionContent): Promise<AddonVersion> {
  return apiFetch(`${BASE}/addons/${enc(addonId)}/versions/${enc(vid)}`, { method: 'PATCH', body: content })
}

export async function publishAddonVersion(addonId: string, vid: string): Promise<AddonVersion> {
  return apiFetch(`${BASE}/addons/${enc(addonId)}/versions/${enc(vid)}/publish`, {
    method: 'POST',
    headers: withIdempotencyKey(),
  })
}
