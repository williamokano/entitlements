/**
 * Billing presentation + state-machine helpers.
 *
 * The lifecycle-action gating below MIRRORS the backend invoice state machine
 * (internal/modules/billing/internal/domain/invoice.go `transitions`): only the
 * transitions legal from the current status are offered in the UI. The backend
 * is still the authority — an illegal transition it rejects surfaces as a 409
 * conflict toast — but we never render an action the machine would refuse.
 *
 * Money is ALWAYS integer minor units + a currency code, never a float; the
 * formatting helper is re-exported from the catalog helpers, which divide by the
 * currency's fraction base only at the very last (Intl) step.
 */

import type { InvoiceStatus, Transition } from './api'

// Money + date formatting are shared with the catalog screens — same golden
// rule (minor units in, format only at display).
export { formatMinor, formatDate, errorMessage } from '../catalog/helpers'

/**
 * Legal lifecycle transitions per status, mirroring the domain `transitions` map:
 *   draft -> void
 *   open  -> pay, void, uncollectible
 *   paid / void / uncollectible -> none (terminal)
 * Note: `open` (issue) is not a per-invoice transition — issuing creates a fresh
 * invoice via POST /invoices, so it is not offered here.
 */
const ACTIONS_BY_STATUS: Record<InvoiceStatus, Transition[]> = {
  draft: ['void'],
  open: ['pay', 'void', 'uncollectible'],
  paid: [],
  void: [],
  uncollectible: [],
}

/** The terminal states — nothing more can be done to the invoice. */
export const isTerminal = (status: InvoiceStatus): boolean =>
  status === 'paid' || status === 'void' || status === 'uncollectible'

/** Lifecycle transitions legal from the given status (empty for terminal states). */
export const allowedActions = (status: InvoiceStatus): Transition[] => ACTIONS_BY_STATUS[status] ?? []

/**
 * Whether a credit note can be raised against an invoice. The backend rejects a
 * credit on a draft or void invoice (service.CreateCreditNote), so we only offer
 * the affordance for open / paid / uncollectible invoices.
 */
export const canCredit = (status: InvoiceStatus): boolean => status !== 'draft' && status !== 'void'

export const STATUS_LABELS: Record<InvoiceStatus, string> = {
  draft: 'Draft',
  open: 'Open',
  paid: 'Paid',
  void: 'Void',
  uncollectible: 'Uncollectible',
}

export const ACTION_LABELS: Record<Transition, string> = {
  pay: 'Mark paid',
  void: 'Void',
  uncollectible: 'Mark uncollectible',
}

/** Tailwind badge classes for an invoice status chip. */
export const statusChipClass = (status: InvoiceStatus): string => {
  switch (status) {
    case 'paid':
      return 'bg-success/15 text-success'
    case 'open':
      return 'bg-info/15 text-info'
    case 'uncollectible':
      return 'bg-danger/15 text-danger'
    case 'void':
      return 'bg-default-200 text-default-600'
    case 'draft':
    default:
      return 'bg-warning/15 text-warning'
  }
}

/** Human-facing invoice number, e.g. 42 -> "INV-000042". */
export const invoiceNumber = (n: number): string => `INV-${String(n).padStart(6, '0')}`

/** Human-facing credit-note number, e.g. 7 -> "CN-000007". */
export const creditNoteNumber = (n: number): string => `CN-${String(n).padStart(6, '0')}`
