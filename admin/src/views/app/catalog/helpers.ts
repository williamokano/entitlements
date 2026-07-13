/**
 * Shared catalog helpers. The golden rule here is the same as the backend's:
 * money is integer minor units + a 3-letter currency code, NEVER a float. The
 * editors keep amounts as integer minor units end-to-end; formatting for display
 * only divides by 100 at the very last step (Intl), it never round-trips a float
 * back into the model.
 */

import { ApiError } from '@/lib/api'
import type { BillingCycle, Price, VersionStatus } from './api'

export const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

export const CYCLE_LABELS: Record<BillingCycle, string> = {
  monthly: 'Monthly',
  annual: 'Annual',
  custom: 'Custom',
}

/**
 * Format integer minor units for display, e.g. (1999, "USD") -> "$19.99". Falls
 * back to a plain "<major>.<minor> CODE" when the currency code is not a valid
 * ISO code Intl recognises. The input MUST be an integer number of minor units.
 */
export const formatMinor = (amountMinor: number, currency: string): string => {
  const code = (currency || '').toUpperCase()
  if (/^[A-Z]{3}$/.test(code)) {
    try {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency: code }).format(amountMinor / 100)
    } catch {
      // Unknown but well-formed code — fall through to the manual format.
    }
  }
  const major = Math.trunc(amountMinor / 100)
  const minor = Math.abs(amountMinor % 100)
    .toString()
    .padStart(2, '0')
  return `${major}.${minor}${code ? ` ${code}` : ''}`
}

/** A compact "$19.99/mo · $199.00/yr" summary of a version's prices. */
export const priceSummary = (prices: Price[], currency: string): string => {
  if (prices.length === 0) return 'No prices'
  const suffix: Record<BillingCycle, string> = { monthly: '/mo', annual: '/yr', custom: '' }
  return prices.map((p) => `${formatMinor(p.amount_minor, currency)}${suffix[p.cycle]}`).join(' · ')
}

/**
 * Parse a user-entered minor-units string into an integer, or return null when
 * it is empty or not a whole number. This is the single gate that keeps floats
 * (and blanks) out of the pricing model.
 */
export const parseMinorUnits = (raw: string): number | null => {
  const trimmed = raw.trim()
  if (trimmed === '') return null
  if (!/^-?\d+$/.test(trimmed)) return null
  const n = Number(trimmed)
  return Number.isSafeInteger(n) ? n : null
}

/** Parse a plain non-negative integer string (grace/trial days, delta amounts). */
export const parseInteger = (raw: string): number | null => {
  const trimmed = raw.trim()
  if (trimmed === '') return null
  if (!/^-?\d+$/.test(trimmed)) return null
  const n = Number(trimmed)
  return Number.isSafeInteger(n) ? n : null
}

/** Tailwind badge classes for a plan/addon lifecycle status. */
export const statusBadgeClass = (status: string): string => {
  switch (status) {
    case 'active':
      return 'bg-success/15 text-success'
    case 'archived':
      return 'bg-default-200 text-default-600'
    case 'draft':
    default:
      return 'bg-warning/15 text-warning'
  }
}

/** Tailwind badge classes for a version status. */
export const versionBadgeClass = (status: VersionStatus): string =>
  status === 'published' ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'

export const formatDate = (value: string | null): string => {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}
