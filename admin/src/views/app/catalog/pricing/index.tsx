// Public pricing preview — adapted from the theme's pages/pricing, bound to the
// open GET /api/v1/catalog/public endpoint. It renders the published, public
// plans exactly as a prospective customer would see them, so operators can sanity
// check the storefront. All amounts are integer minor units, formatted for
// display only at the edge (never parsed back into a float).
import { useCallback, useEffect, useState } from 'react'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { listPublicPlans, type PublicPlan } from '../api'
import CatalogTabs from '../components/CatalogTabs'
import { errorMessage, formatMinor } from '../helpers'

const cycleSuffix: Record<string, string> = { monthly: '/mo', annual: '/yr', custom: '' }

const Page = () => {
  const [plans, setPlans] = useState<PublicPlan[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await listPublicPlans(signal)
      if (signal?.aborted) return
      setPlans(result)
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

  return (
    <>
      <PageBreadcrumb title="Catalog" subtitle="Billing" />
      <CatalogTabs />

      <div className="text-center">
        <h3 className="mt-4 mb-1 text-xl font-bold">Public pricing preview</h3>
        <p className="text-default-500 mb-6 text-sm">The published, public plans as prospective customers see them.</p>
      </div>

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
        <p className="text-default-500 py-8 text-center text-sm">Loading pricing…</p>
      ) : plans.length === 0 ? (
        <div className="py-10 text-center">
          <Icon icon="tag" className="text-default-400 mx-auto text-4xl" />
          <h5 className="mt-3 font-medium">No public plans</h5>
          <p className="text-default-500 mt-1 text-sm">Publish a version and make a plan public to see it here.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
          {plans.map(({ plan, version }) => (
            <div key={plan.id} className="card h-full rounded-xl">
              <div className="card-body p-8 text-center">
                <h3 className="mb-1 text-xl font-bold">{plan.name}</h3>
                <code className="text-default-400 text-xs">{plan.key}</code>

                <div className="my-6">
                  {version.prices.length === 0 ? (
                    <p className="text-default-400 text-sm">Contact us</p>
                  ) : (
                    version.prices.map((price) => (
                      <div key={price.cycle}>
                        <span className="text-3xl font-bold">{formatMinor(price.amount_minor, version.currency)}</span>
                        <span className="text-default-400 text-sm">{cycleSuffix[price.cycle] ?? ''}</span>
                      </div>
                    ))
                  )}
                  {version.trial.enabled && <p className="text-success mt-2 text-sm font-medium">{version.trial.days}-day free trial</p>}
                </div>

                {Object.keys(version.feature_grants).length > 0 && (
                  <ul className="text-start text-sm font-medium">
                    {Object.entries(version.feature_grants).map(([key, value]) => (
                      <li key={key} className="mb-2.5 flex items-center">
                        <Icon icon="check" className="text-success me-2.5 shrink-0" />
                        <span>
                          <code className="text-xs">{key}</code>
                          {value !== true && <span className="text-default-500">: {String(value)}</span>}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  )
}

export default Page
