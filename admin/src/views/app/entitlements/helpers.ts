/**
 * Entitlements presentation helpers.
 *
 * The read endpoints (GET /entitlements, GET /entitlements/{key}) return an
 * effective `value` and its winning `source` layer, but NOT the feature's
 * declared type. So value rendering is driven by the value's JSON runtime type
 * (`valueKind`): a boolean is an on/off capability, a number is a limit quota,
 * and anything else (string / object / array) is a free-form config or enum
 * value shown as-is. Source badges reflect the resolution precedence
 * default < plan < addon < override.
 */

import type { EntitlementValue, Source } from './api'

// Errors and dates are formatted the same way across the app.
export { errorMessage, formatDate } from '../catalog/helpers'

/** The four resolution layers, weakest to strongest — for a stable badge order. */
export const SOURCE_ORDER: Source[] = ['default', 'plan', 'addon', 'override']

export const SOURCE_LABELS: Record<Source, string> = {
  default: 'Default',
  plan: 'Plan',
  addon: 'Add-on',
  override: 'Override',
}

/** Tailwind badge classes for a source chip — override stands out as manual. */
export const sourceBadgeClass = (source: Source): string => {
  switch (source) {
    case 'override':
      return 'bg-primary/15 text-primary'
    case 'addon':
      return 'bg-info/15 text-info'
    case 'plan':
      return 'bg-success/15 text-success'
    case 'default':
    default:
      return 'bg-default-200 text-default-600'
  }
}

/** How to render a value, inferred from its JSON runtime type (no type field on the wire). */
export type ValueKind = 'boolean' | 'limit' | 'config'

export const valueKind = (value: EntitlementValue): ValueKind => {
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return 'limit'
  return 'config'
}

/**
 * A plain-text rendering of a value for tables / labels. Booleans become
 * "On"/"Off", numbers are shown as-is, strings verbatim, and any structured
 * config value is compact JSON. Components may render booleans as chips instead
 * (see `valueKind`), but this is the canonical textual form.
 */
export const formatValue = (value: EntitlementValue): string => {
  switch (valueKind(value)) {
    case 'boolean':
      return value ? 'On' : 'Off'
    case 'limit':
      return String(value)
    case 'config':
    default:
      if (value == null) return '—'
      if (typeof value === 'string') return value
      try {
        return JSON.stringify(value)
      } catch {
        return String(value)
      }
  }
}
