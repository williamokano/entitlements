// Billing → Entitlements → per-feature drill-in. Reuses GET
// /api/v1/entitlements/{key} to show one feature's effective value, the source
// layer it won from, and — for a time-bound override — its expiry. Read-only:
// changing the winning value (creating/editing an override) is F-011.
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import { getEntitlement, type Entitlement } from './api'
import SourceBadge from './components/SourceBadge'
import ValueCell from './components/ValueCell'
import { errorMessage, formatDate, SOURCE_LABELS } from './helpers'

const Page = () => {
  const { key = '' } = useParams<{ key: string }>()
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const e = await getEntitlement(key, signal)
        if (signal?.aborted) return
        setEntitlement(e)
        setError(null)
      } catch (err) {
        if (signal?.aborted) return
        setError(errorMessage(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [key],
  )

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  if (loading) {
    return (
      <>
        <PageBreadcrumb title="Entitlement" subtitle="Billing" />
        <p className="text-default-500 py-8 text-center text-sm">Loading entitlement…</p>
      </>
    )
  }

  if (error || !entitlement) {
    return (
      <>
        <PageBreadcrumb title="Entitlement" subtitle="Billing" />
        <div className="mb-4 flex items-center justify-between rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          <span>{error ?? 'Entitlement not found.'}</span>
          <Link to="/entitlements" className="btn btn-sm bg-danger text-white hover:bg-danger-hover">
            Back to entitlements
          </Link>
        </div>
      </>
    )
  }

  return (
    <>
      <PageBreadcrumb title={entitlement.key} subtitle="Billing" />

      <div className="card">
        <div className="card-header flex items-center justify-between">
          <h4 className="card-title">Feature</h4>
          <Link to="/entitlements" className="btn btn-sm bg-light hover:text-primary">
            Back to entitlements
          </Link>
        </div>
        <div className="card-body">
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <div>
              <dt className="text-default-500 text-xs uppercase">Feature key</dt>
              <dd className="mt-1">
                <code className="text-default-600 rounded bg-default-100 px-1.5 py-0.5 text-sm">{entitlement.key}</code>
              </dd>
            </div>
            <div>
              <dt className="text-default-500 text-xs uppercase">Effective value</dt>
              <dd className="mt-1">
                <ValueCell value={entitlement.value} />
              </dd>
            </div>
            <div>
              <dt className="text-default-500 text-xs uppercase">Source</dt>
              <dd className="mt-1 flex items-center gap-2">
                <SourceBadge source={entitlement.source} />
                <span className="text-default-500 text-sm">won from the {SOURCE_LABELS[entitlement.source].toLowerCase()} layer</span>
              </dd>
            </div>
            {entitlement.expires_at && (
              <div>
                <dt className="text-default-500 text-xs uppercase">Expires</dt>
                <dd className="mt-1 text-sm">{formatDate(entitlement.expires_at)}</dd>
              </div>
            )}
          </dl>
        </div>
      </div>
    </>
  )
}

export default Page
