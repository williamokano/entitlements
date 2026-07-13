// Public plan + billing-cycle picker, reused by both the empty-state subscribe
// flow and the change-plan flow. It renders the published, public plans (the
// same GET /catalog/public the pricing preview uses) with a monthly/annual
// toggle; picking a plan submits its pinned version id + the chosen cycle. All
// amounts are integer minor units, formatted only at the edge (never a float).
import { useCallback, useEffect, useMemo, useState } from 'react'
import Icon from '@/components/wrappers/Icon'
import { listPublicPlans, type BillingCycle, type PublicPlan } from '../../catalog/api'
import { CYCLE_LABELS, errorMessage, formatMinor } from '../../catalog/helpers'

const cycleSuffix: Record<BillingCycle, string> = { monthly: '/mo', annual: '/yr', custom: '' }

// The cycles the picker offers a toggle for (custom pricing is not self-serve).
const SELECTABLE_CYCLES: BillingCycle[] = ['monthly', 'annual']

type Props = {
  /** Wording differs between first subscribe and switching an existing plan. */
  mode: 'subscribe' | 'change'
  /** The tenant's current pinned plan version, marked and non-selectable. */
  currentPlanVersionId?: string
  /** Submit the chosen plan version + cycle; rejects surface inline. */
  onSelect: (planVersionId: string, cycle: BillingCycle) => Promise<void>
}

const PlanPicker = ({ mode, currentPlanVersionId, onSelect }: Props) => {
  const [plans, setPlans] = useState<PublicPlan[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [cycle, setCycle] = useState<BillingCycle>('monthly')
  const [submittingId, setSubmittingId] = useState<string | null>(null)

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

  const priceFor = (plan: PublicPlan): number | null => plan.version.prices.find((p) => p.cycle === cycle)?.amount_minor ?? null

  const select = async (plan: PublicPlan) => {
    if (submittingId) return
    setSubmittingId(plan.version.id)
    setError(null)
    try {
      await onSelect(plan.version.id, cycle)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmittingId(null)
    }
  }

  const anyCycleAvailable = useMemo(() => plans.some((p) => p.version.prices.some((pr) => pr.cycle === cycle)), [plans, cycle])

  return (
    <div>
      <div className="mb-6 flex items-center justify-center gap-2" role="group" aria-label="Billing cycle">
        {SELECTABLE_CYCLES.map((c) => (
          <button
            key={c}
            type="button"
            aria-pressed={cycle === c}
            className={`btn btn-sm ${cycle === c ? 'bg-primary text-white hover:bg-primary-hover' : 'bg-light hover:text-primary'}`}
            onClick={() => setCycle(c)}
          >
            {CYCLE_LABELS[c]}
          </button>
        ))}
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-default-500 py-8 text-center text-sm">Loading plans…</p>
      ) : plans.length === 0 ? (
        <div className="py-10 text-center">
          <Icon icon="tag" className="text-default-400 mx-auto text-4xl" />
          <h5 className="mt-3 font-medium">No plans available</h5>
          <p className="text-default-500 mt-1 text-sm">No public plan is published yet — ask an operator to publish one.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
          {plans.map((plan) => {
            const price = priceFor(plan)
            const isCurrent = currentPlanVersionId === plan.version.id
            const busy = submittingId === plan.version.id
            return (
              <div key={plan.plan.id} className={`card h-full rounded-xl ${isCurrent ? 'border-primary border-2' : ''}`}>
                <div className="card-body p-8 text-center">
                  <h3 className="mb-1 text-xl font-bold">{plan.plan.name}</h3>
                  <code className="text-default-400 text-xs">{plan.plan.key}</code>

                  <div className="my-6">
                    {price === null ? (
                      <p className="text-default-400 text-sm">Not offered {CYCLE_LABELS[cycle].toLowerCase()}</p>
                    ) : (
                      <div>
                        <span className="text-3xl font-bold">{formatMinor(price, plan.version.currency)}</span>
                        <span className="text-default-400 text-sm">{cycleSuffix[cycle]}</span>
                      </div>
                    )}
                    {plan.version.trial.enabled && <p className="text-success mt-2 text-sm font-medium">{plan.version.trial.days}-day free trial</p>}
                  </div>

                  {Object.keys(plan.version.feature_grants).length > 0 && (
                    <ul className="mb-6 text-start text-sm font-medium">
                      {Object.entries(plan.version.feature_grants).map(([key, value]) => (
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

                  {isCurrent ? (
                    <span className="btn bg-light w-full cursor-default">Current plan</span>
                  ) : (
                    <button
                      type="button"
                      className="btn bg-primary text-white hover:bg-primary-hover w-full"
                      disabled={price === null || busy}
                      onClick={() => void select(plan)}
                    >
                      {busy ? 'Submitting…' : mode === 'subscribe' ? 'Subscribe' : 'Switch to this plan'}
                    </button>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {!loading && plans.length > 0 && !anyCycleAvailable && (
        <p className="text-default-500 mt-4 text-center text-sm">No plan offers {CYCLE_LABELS[cycle].toLowerCase()} pricing — try another cycle.</p>
      )}
    </div>
  )
}

export default PlanPicker
