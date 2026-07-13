// Add-on detail — the add-on header (status) with an archive action and a
// versions timeline. Draft versions link to the add-on version editor; a "new
// version" action opens a fresh draft.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'react-toastify'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { confirmAction } from '@/lib/confirm'
import { archiveAddon, createAddonVersion, getAddon, type Addon, type AddonVersion } from '../api'
import CatalogTabs from '../components/CatalogTabs'
import { errorMessage, priceSummary, statusBadgeClass, versionBadgeClass } from '../helpers'

const Page = () => {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [addon, setAddon] = useState<Addon | null>(null)
  const [versions, setVersions] = useState<AddonVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const { addon: a, versions: vs } = await getAddon(id, signal)
        if (signal?.aborted) return
        setAddon(a)
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

  const isArchived = addon?.status === 'archived'

  const onArchive = async () => {
    if (!addon || busy) return
    const ok = await confirmAction({
      title: 'Archive add-on?',
      text: `"${addon.name}" will be archived and can no longer be attached to subscriptions.`,
      confirmText: 'Archive',
    })
    if (!ok) return
    setBusy(true)
    try {
      await archiveAddon(addon.id)
      toast.success('Add-on archived.')
      await load()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onNewVersion = async () => {
    if (!addon || busy) return
    setBusy(true)
    try {
      const version = await createAddonVersion(addon.id)
      navigate(`/catalog/addons/${addon.id}/versions/${version.id}`)
    } catch (err) {
      toast.error(errorMessage(err))
      setBusy(false)
    }
  }

  return (
    <>
      <PageBreadcrumb title={addon ? addon.name : 'Add-on'} subtitle="Catalog" />
      <CatalogTabs />

      <Link to="/catalog/addons" className="text-default-500 hover:text-primary mb-4 inline-flex items-center gap-1 text-sm">
        <Icon icon="arrow-left" className="text-base" />
        Back to add-ons
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
        <p className="text-default-500 py-8 text-center text-sm">Loading add-on…</p>
      ) : addon ? (
        <>
          <div className="card mb-6">
            <div className="card-body flex flex-wrap items-start justify-between gap-4">
              <div>
                <div className="flex items-center gap-2">
                  <h4 className="text-lg font-semibold">{addon.name}</h4>
                  <span className={`badge badge-label ${statusBadgeClass(addon.status)}`}>{addon.status}</span>
                </div>
                <code className="text-default-500 mt-1 inline-block text-xs">{addon.key}</code>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <button type="button" className="btn btn-sm bg-primary text-white hover:bg-primary-hover" disabled={busy || isArchived} onClick={() => void onNewVersion()}>
                  <Icon icon="plus" className="text-base" />
                  New version
                </button>
                <button type="button" className="btn btn-sm bg-danger text-white hover:bg-danger-hover" disabled={busy || isArchived} onClick={() => void onArchive()}>
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
                              {v.deltas.length > 0 && ` · ${v.deltas.length} delta${v.deltas.length === 1 ? '' : 's'}`}
                            </p>
                          </div>
                          <Link
                            to={`/catalog/addons/${addon.id}/versions/${v.id}`}
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
