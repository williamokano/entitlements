/**
 * Tenant store — the "known tenants" list and the current tenant selection.
 *
 * The backend has no "list my tenants" endpoint (see F-004 follow-up), so the
 * switcher is built over a locally-persisted set of tenants the user has
 * created or successfully loaded, plus a "current tenant id". The api client
 * reads the current id and sends it as `X-Tenant-ID` in header tenant mode.
 *
 * Modelled on tokens.ts: a localStorage-backed store with an in-memory snapshot
 * and a subscribe/get/set surface usable from useSyncExternalStore.
 */

export type KnownTenant = {
  id: string
  name: string
  slug: string
}

type TenantState = {
  tenants: KnownTenant[]
  currentId: string | null
}

const STORAGE_KEY = 'entitlements.tenants'

type Listener = () => void
const listeners = new Set<Listener>()

const emptyState: TenantState = { tenants: [], currentId: null }

const isKnownTenant = (t: unknown): t is KnownTenant =>
  !!t &&
  typeof (t as KnownTenant).id === 'string' &&
  typeof (t as KnownTenant).name === 'string' &&
  typeof (t as KnownTenant).slug === 'string'

const readStorage = (): TenantState => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return emptyState
    const parsed = JSON.parse(raw) as Partial<TenantState>
    const tenants = Array.isArray(parsed.tenants) ? parsed.tenants.filter(isKnownTenant) : []
    const currentId =
      typeof parsed.currentId === 'string' && tenants.some((t) => t.id === parsed.currentId)
        ? parsed.currentId
        : (tenants[0]?.id ?? null)
    return { tenants, currentId }
  } catch {
    return emptyState
  }
}

// In-memory snapshot (stable reference for useSyncExternalStore).
let current: TenantState = readStorage()

const notify = () => listeners.forEach((listener) => listener())

const persist = (next: TenantState) => {
  current = next
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    // Storage unavailable (private mode, …) — state stays in memory only.
  }
  notify()
}

export const getTenantState = (): TenantState => current

export const getKnownTenants = (): KnownTenant[] => current.tenants

export const getCurrentTenantId = (): string | null => current.currentId

export const getCurrentTenant = (): KnownTenant | null =>
  current.tenants.find((t) => t.id === current.currentId) ?? null

/**
 * Add (or refresh) a known tenant. Every tenant the user creates — and any
 * tenant successfully loaded via GET /tenants/{id} — flows through here so the
 * switcher stays populated without a server list endpoint.
 */
export function rememberTenant(tenant: KnownTenant): void {
  const others = current.tenants.filter((t) => t.id !== tenant.id)
  persist({ tenants: [...others, tenant], currentId: current.currentId })
}

/** Set the active tenant; subsequent API calls carry its id as X-Tenant-ID. */
export function setCurrentTenant(id: string): void {
  if (current.currentId === id) return
  persist({ tenants: current.tenants, currentId: id })
}

/**
 * Remember a tenant and make it current in one step (used after create and
 * after a switch that also needs to record the tenant).
 */
export function rememberAndSelect(tenant: KnownTenant): void {
  const others = current.tenants.filter((t) => t.id !== tenant.id)
  persist({ tenants: [...others, tenant], currentId: tenant.id })
}

/**
 * Drop a tenant from the store (after a successful delete). If it was current,
 * fall back to another known tenant (or none, which sends the user to
 * onboarding).
 */
export function forgetTenant(id: string): void {
  const tenants = current.tenants.filter((t) => t.id !== id)
  const currentId = current.currentId === id ? (tenants[0]?.id ?? null) : current.currentId
  persist({ tenants, currentId })
}

/** Subscribe to tenant-store changes; returns an unsubscribe function. */
export function subscribe(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
