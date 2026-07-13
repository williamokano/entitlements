/**
 * Subscription presentation + state-machine helpers.
 *
 * The lifecycle-action gating below MIRRORS the backend state machine
 * (internal/modules/subscription/internal/domain/subscription.go `transitions`):
 * only the events legal from the current state are offered in the UI. The
 * backend is still the authority — an illegal transition it rejects surfaces as
 * a 409 conflict toast — but we never render a button the machine would refuse.
 */

import type { SubscriptionStatus } from './api'

/** The user-triggerable lifecycle actions the screen renders. */
export type LifecycleAction = 'pause' | 'resume' | 'reactivate' | 'cancel'

/**
 * Legal lifecycle actions per status, mirroring the domain `transitions` map:
 *   active     -> pause, cancel
 *   paused     -> resume, cancel
 *   suspended  -> reactivate, cancel
 *   trialing / past_due / grace -> cancel
 *   canceled / expired          -> none (terminal)
 * Note: cancel-at-period-end (a non-immediate cancel) is not a state transition,
 * so it stays available from any live state via the cancel modal.
 */
const ACTIONS_BY_STATUS: Record<SubscriptionStatus, LifecycleAction[]> = {
  trialing: ['cancel'],
  active: ['pause', 'cancel'],
  past_due: ['cancel'],
  grace: ['cancel'],
  suspended: ['reactivate', 'cancel'],
  paused: ['resume', 'cancel'],
  canceled: [],
  expired: [],
}

/** The terminal states — nothing more can be done to the subscription. */
export const isTerminal = (status: SubscriptionStatus): boolean => status === 'canceled' || status === 'expired'

/** Lifecycle actions legal from the given status (empty for terminal states). */
export const allowedActions = (status: SubscriptionStatus): LifecycleAction[] => ACTIONS_BY_STATUS[status] ?? []

/**
 * Whether plan changes / addon edits are offered. These operate on a live
 * subscription, so they are hidden once it reaches a terminal state.
 */
export const canModifyPlan = (status: SubscriptionStatus): boolean => !isTerminal(status)

export const STATUS_LABELS: Record<SubscriptionStatus, string> = {
  trialing: 'Trialing',
  active: 'Active',
  past_due: 'Past due',
  grace: 'Grace period',
  suspended: 'Suspended',
  paused: 'Paused',
  canceled: 'Canceled',
  expired: 'Expired',
}

/** Tailwind badge classes for a subscription status chip. */
export const statusChipClass = (status: SubscriptionStatus): string => {
  switch (status) {
    case 'active':
      return 'bg-success/15 text-success'
    case 'trialing':
      return 'bg-info/15 text-info'
    case 'past_due':
    case 'grace':
      return 'bg-warning/15 text-warning'
    case 'suspended':
    case 'canceled':
      return 'bg-danger/15 text-danger'
    case 'paused':
    case 'expired':
    default:
      return 'bg-default-200 text-default-600'
  }
}

export const ACTION_LABELS: Record<LifecycleAction, string> = {
  pause: 'Pause',
  resume: 'Resume',
  reactivate: 'Reactivate',
  cancel: 'Cancel subscription',
}

/**
 * Whole days from now until `iso` (rounded up), or null when there is no date.
 * Used for the trial countdown; negative results clamp to 0.
 */
export const daysUntil = (iso: string | undefined): number | null => {
  if (!iso) return null
  const target = new Date(iso).getTime()
  if (Number.isNaN(target)) return null
  const diffMs = target - Date.now()
  return Math.max(0, Math.ceil(diffMs / 86_400_000))
}
