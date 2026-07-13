// Billing → Entitlements. A read-only view of the current tenant's EFFECTIVE
// entitlements, resolved server-side across the layers default < plan < addon <
// override and returned in a SINGLE GET /api/v1/entitlements call. Each row shows
// the feature key, its effective value (booleans as on/off chips, limits as
// numbers, config/enum verbatim), and a source badge naming the winning layer. A
// tenant with no subscription still shows its `default`-sourced rows. Rows link
// to a per-feature drill-in (GET /api/v1/entitlements/{key}). No mutation here —
// overrides CRUD is F-011 and the usage/quota panel is F-012.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { listEntitlements, type EntitlementRow } from './api'
import SourceBadge from './components/SourceBadge'
import ValueCell from './components/ValueCell'
import { errorMessage } from './helpers'

const Page = () => {
  const navigate = useNavigate()
  const [rows, setRows] = useState<EntitlementRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await listEntitlements(signal)
      if (signal?.aborted) return
      setRows(result)
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

  const isEmpty = !loading && rows.length === 0
  const drillIn = (key: string) => navigate(`/entitlements/${encodeURIComponent(key)}`)

  return (
    <>
      <PageBreadcrumb title="Entitlements" subtitle="Billing" />

      <div className="card">
        <div className="card-header">
          <h4 className="card-title">Effective entitlements</h4>
          <p className="text-default-500 mt-1 text-sm">
            What this tenant can do right now, resolved across default, plan, add-on, and override layers. The source badge names the winning layer.
          </p>
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
            <p className="text-default-500 py-8 text-center text-sm">Loading entitlements…</p>
          ) : isEmpty ? (
            <div className="py-10 text-center">
              <Icon icon="components" className="text-default-400 mx-auto text-4xl" />
              <h5 className="mt-3 font-medium">No entitlements</h5>
              <p className="text-default-500 mt-1 text-sm">No features are defined for this tenant yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="table w-full">
                <thead>
                  <tr>
                    <th className="text-start">Feature</th>
                    <th className="text-start">Value</th>
                    <th className="text-start">Source</th>
                    <th className="text-end">Details</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.key} className="hover:bg-default-50 cursor-pointer" onClick={() => drillIn(row.key)}>
                      <td className="font-medium">
                        <button
                          type="button"
                          className="text-primary hover:underline"
                          aria-label={`Open ${row.key}`}
                          onClick={(e) => {
                            e.stopPropagation()
                            drillIn(row.key)
                          }}
                        >
                          {row.key}
                        </button>
                      </td>
                      <td>
                        <ValueCell value={row.value} />
                      </td>
                      <td>
                        <SourceBadge source={row.source} />
                      </td>
                      <td className="text-end">
                        <Icon icon="chevron-right" className="text-default-400" />
                      </td>
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
