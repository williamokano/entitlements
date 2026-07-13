/**
 * Typed client for the tenant-scoped subscription surface
 * (`/api/v1/subscription`). Every route requires the bearer token AND the
 * current tenant (the shared api client attaches `X-Tenant-ID` in header mode) —
 * a subscription is per-tenant, so the active tenant selects which one.
 *
 * Backend JSON shapes (see internal/modules/subscription/internal/adapters/rest/handler.go):
 *   GET    /subscription                     -> Subscription  (404 when the tenant has none)
 *   POST   /subscription {plan_version_id,cycle}     -> Subscription (201)
 *   POST   /subscription/cancel {immediate,reason?}  -> Subscription
 *   POST   /subscription/{pause,resume,reactivate} {reason?} -> Subscription
 *   POST   /subscription/change-plan {plan_version_id,cycle}  -> Subscription
 *          (immediate upgrade re-pins; a downgrade returns a scheduled_change)
 *   POST   /subscription/scheduled-change/cancel     -> Subscription
 *   POST   /subscription/addons {addon_version_id,quantity} -> Subscription
 *   DELETE /subscription/addons/{vid}                -> Subscription
 *
 * Money is never handled here — amounts for display come from the catalog
 * (integer minor units + currency), reused via ../catalog/helpers.
 */

import { ApiError, apiFetch, withIdempotencyKey } from '@/lib/api'
import type { BillingCycle } from '../catalog/api'

export type SubscriptionStatus =
  | 'trialing'
  | 'active'
  | 'past_due'
  | 'grace'
  | 'suspended'
  | 'paused'
  | 'canceled'
  | 'expired'

/** A pending plan change that applies at the current period's end (a downgrade). */
export type ScheduledChange = {
  plan_version_id: string
  billing_cycle: BillingCycle
}

/** An addon attached to the subscription, at a given quantity. */
export type SubscriptionAddon = {
  addon_version_id: string
  quantity: number
}

export type Subscription = {
  id: string
  plan_version_id: string
  billing_cycle: BillingCycle
  status: SubscriptionStatus
  current_period_start: string
  current_period_end: string
  trial_ends_at?: string
  cancel_at_period_end: boolean
  scheduled_change?: ScheduledChange
  addons?: SubscriptionAddon[]
}

const BASE = '/api/v1/subscription'
const enc = encodeURIComponent

/**
 * The current tenant's live subscription, or `null` when it has none (the
 * backend answers 404 — the empty state, not an error, so we swallow it).
 */
export async function getSubscription(signal?: AbortSignal): Promise<Subscription | null> {
  try {
    return await apiFetch<Subscription>(BASE, { signal })
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

export async function subscribe(planVersionID: string, cycle: BillingCycle): Promise<Subscription> {
  return apiFetch(BASE, {
    method: 'POST',
    body: { plan_version_id: planVersionID, cycle },
    headers: withIdempotencyKey(),
  })
}

export async function cancelSubscription(immediate: boolean, reason?: string): Promise<Subscription> {
  return apiFetch(`${BASE}/cancel`, { method: 'POST', body: { immediate, reason: reason ?? '' } })
}

export async function pauseSubscription(reason?: string): Promise<Subscription> {
  return apiFetch(`${BASE}/pause`, { method: 'POST', body: { reason: reason ?? '' } })
}

export async function resumeSubscription(reason?: string): Promise<Subscription> {
  return apiFetch(`${BASE}/resume`, { method: 'POST', body: { reason: reason ?? '' } })
}

export async function reactivateSubscription(reason?: string): Promise<Subscription> {
  return apiFetch(`${BASE}/reactivate`, { method: 'POST', body: { reason: reason ?? '' } })
}

export async function changePlan(planVersionID: string, cycle: BillingCycle): Promise<Subscription> {
  return apiFetch(`${BASE}/change-plan`, {
    method: 'POST',
    body: { plan_version_id: planVersionID, cycle },
    headers: withIdempotencyKey(),
  })
}

export async function cancelScheduledChange(): Promise<Subscription> {
  return apiFetch(`${BASE}/scheduled-change/cancel`, { method: 'POST' })
}

export async function attachAddon(addonVersionID: string, quantity: number): Promise<Subscription> {
  return apiFetch(`${BASE}/addons`, {
    method: 'POST',
    body: { addon_version_id: addonVersionID, quantity },
    headers: withIdempotencyKey(),
  })
}

export async function detachAddon(addonVersionID: string): Promise<Subscription> {
  return apiFetch(`${BASE}/addons/${enc(addonVersionID)}`, { method: 'DELETE' })
}
