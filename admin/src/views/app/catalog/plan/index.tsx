// Plan detail — the plan header (status + public badge) with lifecycle actions
// (public toggle, archive), plus a versions timeline. Draft versions link to the
// editor; a "new version" action clones the latest into a fresh draft. Archiving
// is gated behind a confirm; the public toggle flips visibility in place.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'react-toastify'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { confirmAction } from '@/lib/confirm'
import { archivePlan, createPlanVersion, getPlan, setPlanPublic, type Plan, type PlanVersion } from '../api'
import CatalogTabs from '../components/CatalogTabs'
import { errorMessage, formatDate, priceSummary, statusBadgeClass, versionBadgeClass } from '../helpers'

const Page = () => {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [plan, setPlan] = useState<Plan | null>(null)
  const [versions, setVersions] = useState<PlanVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const { plan: p, versions: vs } = await getPlan(id, signal)
        if (signal?.aborted) return
        setPlan(p)
        setVersions(vs)
        setError(null)
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

  const isArchived = plan?.status === 'archived'

  const togglePublic = async () => {
    if (!plan || busy) return
    setBusy(true)
    try {
      const updated = await setPlanPublic(plan.id, !plan.public)
      setPlan(updated)
      toast.success(updated.public ? 'Plan is now public.' : 'Plan is now private.')
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onArchive = async () => {
    if (!plan || busy) return
    const ok = await confirmAction({
      title: 'Archive plan?',
      text: `"${plan.name}" will be archived and can no longer be offered to new subscribers.`,
      confirmText: 'Archive',
    })
    if (!ok) return
    setBusy(true)
    try {
      await archivePlan(plan.id)
      toast.success('Plan archived.')
      await load()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onNewVersion = async () => {
    if (!plan || busy) return
    setBusy(true)
    try {
      const version = await createPlanVersion(plan.id)
      navigate(`/catalog/plans/${plan.id}/versions/${version.id}`)
    } catch (err) {
      toast.error(errorMessage(err))
      setBusy(false)
    }
  }

  return (
    <>
      <PageBreadcrumb title={plan ? plan.name : 'Plan'} subtitle="Catalog" />
      <CatalogTabs />

      <Link to="/catalog" className="text-default-500 hover:text-primary mb-4 inline-flex items-center gap-1 text-sm">
        <Icon icon="arrow-left" className="text-base" />
        Back to plans
      </Link>

      {error && (
        <div className="mb-4 flex items-center justify-between rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          <span>{error}</span>
          <button
            type="button"
            className="btn btn-sm bg-danger text-white hover:bg-danger-hover"
            onClick={() => {
              setLoading(true)
              setError(null)
              void load()
            }}
          >
            Retry
          </button>
        </div>
      )}

      {loading ? (
        <p className="text-default-500 py-8 text-center text-sm">Loading plan…</p>
      ) : plan ? (
        <>
          <div className="card mb-6">
            <div className="card-body flex flex-wrap items-start justify-between gap-4">
              <div>
                <div className="flex items-center gap-2">
                  <h4 className="text-lg font-semibold">{plan.name}</h4>
                  <span className={`badge badge-label ${statusBadgeClass(plan.status)}`}>{plan.status}</span>
                  {plan.public ? (
                    <span className="badge badge-label bg-info/15 text-info">Public</span>
                  ) : (
                    <span className="badge badge-label bg-default-200 text-default-600">Private</span>
                  )}
                </div>
                <code className="text-default-500 mt-1 inline-block text-xs">{plan.key}</code>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  className="btn btn-sm border border-default-300 hover:border-primary hover:text-primary"
                  disabled={busy || isArchived}
                  onClick={() => void togglePublic()}
                >
                  {plan.public ? 'Make private' : 'Make public'}
                </button>
                <button
                  type="button"
                  className="btn btn-sm bg-primary text-white hover:bg-primary-hover"
                  disabled={busy || isArchived}
                  onClick={() => void onNewVersion()}
                >
                  <Icon icon="plus" className="text-base" />
                  New version
                </button>
                <button
                  type="button"
                  className="btn btn-sm bg-danger text-white hover:bg-danger-hover"
                  disabled={busy || isArchived}
                  onClick={() => void onArchive()}
                >
                  Archive
                </button>
              </div>
            </div>
          </div>

          <div className="card">
            <div className="card-header">
              <h4 className="card-title">Versions</h4>
            </div>
            <div className="card-body">
              {versions.length === 0 ? (
                <p className="text-default-500 py-6 text-center text-sm">No versions yet.</p>
              ) : (
                <ol className="relative space-y-4 border-s border-default-200 ps-6">
                  {[...versions]
                    .sort((a, b) => b.version - a.version)
                    .map((v) => (
                      <li key={v.id} className="relative">
                        <span className="absolute -start-[1.65rem] mt-1 flex h-3 w-3 items-center justify-center rounded-full bg-primary" aria-hidden />
                        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-default-200 p-4">
                          <div>
                            <div className="flex items-center gap-2">
                              <span className="font-medium">Version {v.version}</span>
                              <span className={`badge badge-label ${versionBadgeClass(v.status)}`}>{v.status}</span>
                            </div>
                            <p className="text-default-500 mt-1 text-sm">
                              {v.currency ? `${priceSummary(v.prices, v.currency)} · ${v.currency}` : 'Pricing not set'}
                            </p>
                            {v.published_at && <p className="text-default-400 mt-0.5 text-xs">Published {formatDate(v.published_at)}</p>}
                          </div>
                          <Link
                            to={`/catalog/plans/${plan.id}/versions/${v.id}`}
                            className="btn btn-sm border border-default-300 hover:border-primary hover:text-primary"
                          >
                            {v.status === 'draft' ? 'Edit' : 'View'}
                          </Link>
                        </div>
                      </li>
                    ))}
                </ol>
              )}
            </div>
          </div>
        </>
      ) : null}
    </>
  )
}

export default Page
