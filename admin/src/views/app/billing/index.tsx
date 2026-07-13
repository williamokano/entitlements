// Billing → Invoices list. Loads the current tenant's invoices and renders them
// in a table (number, status, issued date, currency, total). Totals are rendered
// from integer minor units formatted to the invoice currency — never parsed into
// a float. Rows link to the invoice detail. An "Issue invoice" action snapshots
// the tenant's live subscription into a fresh invoice (draft→open) via
// POST /billing/invoices.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'react-toastify'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { issueInvoice, listInvoices, type Invoice } from './api'
import { errorMessage, formatDate, formatMinor, invoiceNumber, STATUS_LABELS, statusChipClass } from './helpers'

const Page = () => {
  const navigate = useNavigate()
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [issuing, setIssuing] = useState(false)

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await listInvoices(signal)
      if (signal?.aborted) return
      setInvoices(result)
      setError(null)
    } catch (err) {
      if (signal?.aborted) return
      setError(errorMessage(err))
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  const retry = () => {
    setLoading(true)
    setError(null)
    void load()
  }

  const onIssue = async () => {
    if (issuing) return
    setIssuing(true)
    try {
      const created = await issueInvoice()
      toast.success(`${invoiceNumber(created.number)} issued.`)
      navigate(`/billing/invoices/${created.id}`)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        toast.error(err.detail || err.title || 'That action is not allowed right now.')
      } else {
        toast.error(errorMessage(err))
      }
    } finally {
      setIssuing(false)
    }
  }

  const isEmpty = !loading && invoices.length === 0

  return (
    <>
      <PageBreadcrumb title="Invoices" subtitle="Billing" />

      <div className="card">
        <div className="card-header flex items-center justify-between">
          <div>
            <h4 className="card-title">Invoices</h4>
            <p className="text-default-500 mt-1 text-sm">Invoices issued to this tenant. Totals are shown in each invoice's currency.</p>
          </div>
          <button type="button" className="btn bg-primary text-white hover:bg-primary-hover" onClick={() => void onIssue()} disabled={issuing}>
            <Icon icon="plus" className="text-base" />
            {issuing ? 'Issuing…' : 'Issue invoice'}
          </button>
        </div>

        <div className="card-body">
          {error && (
            <div className="mb-4 flex items-center justify-between rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
              <span>{error}</span>
              <button type="button" className="btn btn-sm bg-danger text-white hover:bg-danger-hover" onClick={retry}>
                Retry
              </button>
            </div>
          )}

          {loading ? (
            <p className="text-default-500 py-8 text-center text-sm">Loading invoices…</p>
          ) : isEmpty ? (
            <div className="py-10 text-center">
              <Icon icon="invoice" className="text-default-400 mx-auto text-4xl" />
              <h5 className="mt-3 font-medium">No invoices yet</h5>
              <p className="text-default-500 mt-1 text-sm">Issue an invoice to snapshot the tenant's current subscription.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="table w-full">
                <thead>
                  <tr>
                    <th className="text-start">Number</th>
                    <th className="text-start">Status</th>
                    <th className="text-start">Issued</th>
                    <th className="text-start">Currency</th>
                    <th className="text-end">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {invoices.map((inv) => (
                    <tr key={inv.id} className="hover:bg-default-50 cursor-pointer" onClick={() => navigate(`/billing/invoices/${inv.id}`)}>
                      <td className="font-medium">
                        <button
                          type="button"
                          className="text-primary hover:underline"
                          aria-label={`Open ${invoiceNumber(inv.number)}`}
                          onClick={(e) => {
                            e.stopPropagation()
                            navigate(`/billing/invoices/${inv.id}`)
                          }}
                        >
                          {invoiceNumber(inv.number)}
                        </button>
                      </td>
                      <td>
                        <span className={`badge badge-label ${statusChipClass(inv.status)}`}>{STATUS_LABELS[inv.status]}</span>
                      </td>
                      <td>{formatDate(inv.issued_at ?? null)}</td>
                      <td>
                        <code className="text-default-600 rounded bg-default-100 px-1.5 py-0.5 text-xs">{inv.currency}</code>
                      </td>
                      <td className="text-end font-medium">{formatMinor(inv.total_minor, inv.currency)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </>
  )
}

export default Page
