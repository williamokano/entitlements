// Billing → Invoice detail. Loads one invoice (with its snapshotted line items)
// plus its credit notes, and renders:
//   • a header with the number, status chip, and issued date;
//   • lifecycle actions (pay / void / mark uncollectible) gated by the backend
//     state machine — only transitions legal from the current status show;
//   • the snapshotted line items (kind, description, key + version, unit price,
//     quantity, amount) and the subtotal / tax / total breakdown;
//   • the credit notes list (amounts are negative — negated invoice amounts) and
//     a "credit note" affordance (amount + mandatory reason).
// All money is rendered from integer minor units — never parsed into a float.
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router'
import { toast } from 'react-toastify'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { confirmAction } from '@/lib/confirm'
import { createCreditNote, getInvoice, listCreditNotes, transitionInvoice, type CreditNote, type Invoice, type Transition } from './api'
import CreditNoteModal from './components/CreditNoteModal'
import {
  ACTION_LABELS,
  allowedActions,
  canCredit,
  creditNoteNumber,
  errorMessage,
  formatDate,
  formatMinor,
  invoiceNumber,
  STATUS_LABELS,
  statusChipClass,
} from './helpers'

const raise409 = (err: unknown): void => {
  if (err instanceof ApiError && err.status === 409) {
    toast.error(err.detail || err.title || 'That action is not allowed right now.')
  } else {
    toast.error(errorMessage(err))
  }
}

const Page = () => {
  const { id = '' } = useParams<{ id: string }>()
  const [invoice, setInvoice] = useState<Invoice | null>(null)
  const [creditNotes, setCreditNotes] = useState<CreditNote[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [showCreditNote, setShowCreditNote] = useState(false)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const inv = await getInvoice(id, signal)
        if (signal?.aborted) return
        setInvoice(inv)
        setError(null)
        // Credit notes are a secondary concern — a failure there must not blank
        // the invoice, so it is swallowed to an empty list.
        try {
          const notes = await listCreditNotes(id, signal)
          if (!signal?.aborted) setCreditNotes(notes)
        } catch {
          if (!signal?.aborted) setCreditNotes([])
        }
      } catch (err) {
        if (signal?.aborted) return
        setError(errorMessage(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [id],
  )

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  const runTransition = async (action: Transition) => {
    if (acting || !invoice) return
    // Void and uncollectible are irreversible (terminal) — confirm first.
    if (action !== 'pay') {
      const ok = await confirmAction({
        title: `${ACTION_LABELS[action]}?`,
        text: `This moves ${invoiceNumber(invoice.number)} to a terminal state and cannot be undone.`,
        confirmText: ACTION_LABELS[action],
      })
      if (!ok) return
    }
    setActing(true)
    try {
      const next = await transitionInvoice(invoice.id, action)
      setInvoice(next)
      toast.success(`${ACTION_LABELS[action]} succeeded.`)
    } catch (err) {
      raise409(err)
    } finally {
      setActing(false)
    }
  }

  const onCreateCreditNote = async (amountMinor: number, reason: string) => {
    if (!invoice) return
    const created = await createCreditNote(invoice.id, amountMinor, reason)
    setCreditNotes((prev) => [created, ...prev])
    setShowCreditNote(false)
    toast.success(`${creditNoteNumber(created.number)} issued.`)
  }

  if (loading) {
    return (
      <>
        <PageBreadcrumb title="Invoice" subtitle="Billing" />
        <p className="text-default-500 py-8 text-center text-sm">Loading invoice…</p>
      </>
    )
  }

  if (error || !invoice) {
    return (
      <>
        <PageBreadcrumb title="Invoice" subtitle="Billing" />
        <div className="mb-4 flex items-center justify-between rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          <span>{error ?? 'Invoice not found.'}</span>
          <Link to="/billing" className="btn btn-sm bg-danger text-white hover:bg-danger-hover">
            Back to invoices
          </Link>
        </div>
      </>
    )
  }

  const actions = allowedActions(invoice.status)
  const lines = invoice.line_items ?? []

  return (
    <>
      <PageBreadcrumb title={invoiceNumber(invoice.number)} subtitle="Billing" />

      <div className="card">
        <div className="card-body">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="mb-2 flex items-center gap-2">
                <h4 className="text-lg font-semibold">{invoiceNumber(invoice.number)}</h4>
                <span className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusChipClass(invoice.status)}`}>{STATUS_LABELS[invoice.status]}</span>
              </div>
              <p className="text-default-500 text-sm">
                Issued: {formatDate(invoice.issued_at ?? null)} · Currency: {invoice.currency}
              </p>
            </div>

            <div className="flex flex-wrap gap-2">
              {actions.map((action) => (
                <button
                  key={action}
                  type="button"
                  className={
                    action === 'pay'
                      ? 'btn btn-sm bg-success text-white hover:opacity-90'
                      : 'btn btn-sm bg-danger/10 text-danger hover:bg-danger hover:text-white'
                  }
                  disabled={acting}
                  onClick={() => void runTransition(action)}
                >
                  {ACTION_LABELS[action]}
                </button>
              ))}
              {canCredit(invoice.status) && (
                <button type="button" className="btn btn-sm bg-primary text-white hover:bg-primary-hover" onClick={() => setShowCreditNote(true)}>
                  Issue credit note
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="card mt-6">
        <div className="card-header">
          <h4 className="card-title">Line items</h4>
        </div>
        <div className="card-body">
          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th className="text-start">Kind</th>
                  <th className="text-start">Description</th>
                  <th className="text-start">Key / version</th>
                  <th className="text-end">Unit price</th>
                  <th className="text-end">Qty</th>
                  <th className="text-end">Amount</th>
                </tr>
              </thead>
              <tbody>
                {lines.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="text-default-400 py-3 text-center">
                      No line items.
                    </td>
                  </tr>
                ) : (
                  lines.map((li, idx) => (
                    <tr key={`${li.key}-${li.version}-${idx}`}>
                      <td>
                        <span className="badge badge-label bg-default-200 text-default-600 capitalize">{li.kind}</span>
                      </td>
                      <td>{li.description}</td>
                      <td>
                        <code className="text-default-600 rounded bg-default-100 px-1.5 py-0.5 text-xs">
                          {li.key} v{li.version}
                        </code>
                      </td>
                      <td className="text-end">{formatMinor(li.unit_price_minor, li.currency)}</td>
                      <td className="text-end">{li.quantity}</td>
                      <td className="text-end font-medium">{formatMinor(li.amount_minor, li.currency)}</td>
                    </tr>
                  ))
                )}
              </tbody>
              <tfoot>
                <tr>
                  <td colSpan={5} className="text-end text-default-500">
                    Subtotal
                  </td>
                  <td className="text-end">{formatMinor(invoice.subtotal_minor, invoice.currency)}</td>
                </tr>
                <tr>
                  <td colSpan={5} className="text-end text-default-500">
                    Tax
                  </td>
                  <td className="text-end">{formatMinor(invoice.tax_minor, invoice.currency)}</td>
                </tr>
                <tr>
                  <td colSpan={5} className="text-end font-semibold">
                    Total
                  </td>
                  <td className="text-end font-semibold">{formatMinor(invoice.total_minor, invoice.currency)}</td>
                </tr>
              </tfoot>
            </table>
          </div>
        </div>
      </div>

      <div className="card mt-6">
        <div className="card-header flex items-center justify-between">
          <div>
            <h4 className="card-title">Credit notes</h4>
            <p className="text-default-500 mt-1 text-sm">Credits reduce what the tenant owes — amounts are shown negated.</p>
          </div>
          {canCredit(invoice.status) && (
            <button type="button" className="btn btn-sm bg-light hover:text-primary" onClick={() => setShowCreditNote(true)}>
              <Icon icon="plus" className="text-base" />
              Add credit note
            </button>
          )}
        </div>
        <div className="card-body">
          {creditNotes.length === 0 ? (
            <p className="text-default-500 py-4 text-center text-sm">No credit notes for this invoice.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="table w-full">
                <thead>
                  <tr>
                    <th className="text-start">Number</th>
                    <th className="text-start">Reason</th>
                    <th className="text-end">Amount</th>
                  </tr>
                </thead>
                <tbody>
                  {creditNotes.map((cn) => (
                    <tr key={cn.id}>
                      <td className="font-medium">{creditNoteNumber(cn.number)}</td>
                      <td>{cn.reason}</td>
                      <td className="text-end font-medium text-danger">{formatMinor(cn.amount_minor, cn.currency)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {showCreditNote && <CreditNoteModal currency={invoice.currency} onClose={() => setShowCreditNote(false)} onSubmit={onCreateCreditNote} />}
    </>
  )
}

export default Page
