/**
 * Typed client for the tenant-scoped billing surface
 * (`/api/v1/billing`). Every route requires the bearer token AND the current
 * tenant (the shared api client attaches `X-Tenant-ID` in header mode) — invoices
 * are per-tenant, so the active tenant selects which set is returned.
 *
 * Backend JSON shapes (see internal/modules/billing/internal/adapters/rest/handler.go
 * — invoiceResponse / creditNoteResponse):
 *   GET  /billing/invoices                       -> { invoices: Invoice[] }
 *   GET  /billing/invoices/{id}                  -> Invoice (with line_items)
 *   POST /billing/invoices                       -> Invoice (201, snapshots the live sub)
 *   POST /billing/invoices/{id}/pay              -> Invoice
 *   POST /billing/invoices/{id}/void             -> Invoice
 *   POST /billing/invoices/{id}/uncollectible    -> Invoice
 *   GET  /billing/invoices/{id}/credit-notes     -> { credit_notes: CreditNote[] }
 *   POST /billing/invoices/{id}/credit-notes {amount_minor,reason} -> CreditNote (201)
 *
 * Money is ALWAYS integer minor units + a currency code — never a float. Amounts
 * are formatted for display only at the very last step (see helpers.formatMinor);
 * the credit note `amount_minor` arrives already negated (negative) from the
 * backend and is rendered as-is.
 */

import { apiFetch, withIdempotencyKey } from '@/lib/api'

/** Invoice lifecycle state — mirrors the backend domain.Status. */
export type InvoiceStatus = 'draft' | 'open' | 'paid' | 'void' | 'uncollectible'

/** A snapshotted charge line on an invoice. Every field is a copy taken at issuance. */
export type LineItem = {
  kind: 'plan' | 'addon'
  description: string
  key: string
  version: number
  unit_price_minor: number
  quantity: number
  amount_minor: number
  currency: string
}

export type Invoice = {
  id: string
  number: number
  subscription_id: string
  status: InvoiceStatus
  currency: string
  subtotal_minor: number
  tax_minor: number
  total_minor: number
  /** Present only once the invoice has been issued (draft invoices omit it). */
  issued_at?: string
  /** Present on the detail response; may be absent on a list row. */
  line_items?: LineItem[]
}

/** A credit note negating (part of) an invoice's amount. `amount_minor` is negative. */
export type CreditNote = {
  id: string
  invoice_id: string
  number: number
  amount_minor: number
  currency: string
  reason: string
}

const BASE = '/api/v1/billing/invoices'
const enc = encodeURIComponent

/** The current tenant's invoices, newest number first (backend ordering). */
export async function listInvoices(signal?: AbortSignal): Promise<Invoice[]> {
  const res = await apiFetch<{ invoices: Invoice[] }>(BASE, { signal })
  return res.invoices ?? []
}

/** One invoice (with its snapshotted line items), scoped to the current tenant. */
export async function getInvoice(id: string, signal?: AbortSignal): Promise<Invoice> {
  return apiFetch<Invoice>(`${BASE}/${enc(id)}`, { signal })
}

/** Issue a new invoice snapshotting the tenant's live subscription (draft→open). */
export async function issueInvoice(): Promise<Invoice> {
  return apiFetch<Invoice>(BASE, { method: 'POST', headers: withIdempotencyKey() })
}

/** The per-invoice lifecycle transitions the backend exposes. */
export type Transition = 'pay' | 'void' | 'uncollectible'

/** Apply a lifecycle transition to an invoice; returns the updated invoice. */
export async function transitionInvoice(id: string, action: Transition): Promise<Invoice> {
  return apiFetch<Invoice>(`${BASE}/${enc(id)}/${action}`, { method: 'POST' })
}

/** An invoice's credit notes (amounts are negative — negated invoice amounts). */
export async function listCreditNotes(id: string, signal?: AbortSignal): Promise<CreditNote[]> {
  const res = await apiFetch<{ credit_notes: CreditNote[] }>(`${BASE}/${enc(id)}/credit-notes`, { signal })
  return res.credit_notes ?? []
}

/**
 * Create a credit note against an invoice. `amountMinor` is the POSITIVE
 * magnitude to credit (integer minor units); the backend stores the negation.
 * A reason is mandatory (the backend rejects an empty one with 400).
 */
export async function createCreditNote(id: string, amountMinor: number, reason: string): Promise<CreditNote> {
  return apiFetch<CreditNote>(`${BASE}/${enc(id)}/credit-notes`, {
    method: 'POST',
    body: { amount_minor: amountMinor, reason },
    headers: withIdempotencyKey(),
  })
}
