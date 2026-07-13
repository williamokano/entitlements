// Add-ons list — the second catalog section. Add-ons extend plans with extra
// pricing and entitlement deltas; like plans, their content lives in versions.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { createAddon, listAddons, type Addon } from '../api'
import CatalogTabs from '../components/CatalogTabs'
import CreateItemModal from '../components/CreateItemModal'
import { errorMessage, statusBadgeClass } from '../helpers'

const Page = () => {
  const navigate = useNavigate()
  const [addons, setAddons] = useState<Addon[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await listAddons(signal)
      if (signal?.aborted) return
      setAddons(result)
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

  const handleCreate = async (key: string, name: string) => {
    const { addon } = await createAddon(key, name)
    setCreating(false)
    navigate(`/catalog/addons/${addon.id}`)
  }

  const isEmpty = !loading && addons.length === 0

  return (
    <>
      <PageBreadcrumb title="Catalog" subtitle="Billing" />
      <CatalogTabs />

      <div className="card">
        <div className="card-header flex items-center justify-between">
          <div>
            <h4 className="card-title">Add-ons</h4>
            <p className="text-default-500 mt-1 text-sm">Optional extras that layer entitlement deltas on top of a plan.</p>
          </div>
          <button type="button" className="btn bg-primary text-white hover:bg-primary-hover" onClick={() => setCreating(true)}>
            <Icon icon="plus" className="text-base" />
            New add-on
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
            <p className="text-default-500 py-8 text-center text-sm">Loading add-ons…</p>
          ) : isEmpty ? (
            <div className="py-10 text-center">
              <Icon icon="puzzle" className="text-default-400 mx-auto text-4xl" />
              <h5 className="mt-3 font-medium">No add-ons yet</h5>
              <p className="text-default-500 mt-1 text-sm">Create an add-on to offer extras alongside your plans.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="table w-full">
                <thead>
                  <tr>
                    <th className="text-start">Name</th>
                    <th className="text-start">Key</th>
                    <th className="text-start">Status</th>
                    <th className="text-end">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {addons.map((addon) => (
                    <tr key={addon.id} className="hover:bg-default-50 cursor-pointer" onClick={() => navigate(`/catalog/addons/${addon.id}`)}>
                      <td className="font-medium">{addon.name}</td>
                      <td>
                        <code className="text-default-600 rounded bg-default-100 px-1.5 py-0.5 text-xs">{addon.key}</code>
                      </td>
                      <td>
                        <span className={`badge badge-label ${statusBadgeClass(addon.status)}`}>{addon.status}</span>
                      </td>
                      <td className="text-end">
                        <button
                          type="button"
                          className="btn btn-sm border border-default-300 hover:border-primary hover:text-primary"
                          aria-label={`Open ${addon.name}`}
                          onClick={(e) => {
                            e.stopPropagation()
                            navigate(`/catalog/addons/${addon.id}`)
                          }}
                        >
                          Manage
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {creating && <CreateItemModal noun="Add-on" onClose={() => setCreating(false)} onCreate={handleCreate} />}
    </>
  )
}

export default Page
