// Billing → Subscription. Loads the current tenant's live subscription and
// renders one of two worlds:
//   • no subscription (GET returns 404) — a subscribe flow (public plan + cycle
//     picker) that POSTs /subscription;
//   • a live subscription — a status card whose lifecycle actions are gated by
//     the backend state machine (only legal transitions show), a change-plan
//     flow (immediate re-pin vs a scheduled-for-period-end downgrade), and an
//     addons section.
// Errors follow the house rules: an illegal transition / conflict (409) raises a
// conflict toast; validation problems (400/422) render inline in their form.
import { useCallback, useEffect, useState } from 'react'
import { toast } from 'react-toastify'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { getPlanVersion, type BillingCycle, type Price } from '../catalog/api'
import { CYCLE_LABELS, errorMessage, formatDate, formatMinor } from '../catalog/helpers'
import {
  cancelScheduledChange,
  cancelSubscription,
  changePlan,
  getSubscription,
  pauseSubscription,
  reactivateSubscription,
  resumeSubscription,
  subscribe,
  type Subscription,
} from './api'
import AddonsSection from './components/AddonsSection'
import CancelModal from './components/CancelModal'
import PlanPicker from './components/PlanPicker'
import { ACTION_LABELS, allowedActions, canModifyPlan, daysUntil, STATUS_LABELS, statusChipClass, type LifecycleAction } from './helpers'

// The pinned plan version's display data (name key + prices), loaded from the
// catalog so the card can show what the tenant is paying and the addons section
// can filter by compatible plan key. Best-effort — the card renders without it.
type PlanDetail = { planVersionId: string; planKey: string; currency: string; prices: Price[] }

const raise409 = (err: unknown): void => {
  if (err instanceof ApiError && err.status === 409) {
    toast.error(err.detail || err.title || 'That action is not allowed right now.')
  } else {
    toast.error(errorMessage(err))
  }
}

const Page = () => {
  const [sub, setSub] = useState<Subscription | null>(null)
  const [planDetail, setPlanDetail] = useState<PlanDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [showChangePlan, setShowChangePlan] = useState(false)
  const [showCancel, setShowCancel] = useState(false)

  const loadPlanDetail = useCallback(async (planVersionId: string, signal?: AbortSignal) => {
    try {
      const pv = await getPlanVersion(planVersionId, signal)
      if (signal?.aborted) return
      setPlanDetail({ planVersionId, planKey: pv.plan_key, currency: pv.currency, prices: pv.prices })
    } catch {
      if (!signal?.aborted) setPlanDetail(null)
    }
  }, [])

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const result = await getSubscription(signal)
        if (signal?.aborted) return
        setSub(result)
        setError(null)
        if (result) await loadPlanDetail(result.plan_version_id, signal)
        else setPlanDetail(null)
      } catch (err) {
        if (signal?.aborted) return
        setError(errorMessage(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [loadPlanDetail],
  )

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  // Apply a subscription returned by a mutation; refresh the plan detail when
  // the pinned version changed (an immediate upgrade re-pins).
  const applySub = useCallback(
    (next: Subscription) => {
      setSub(next)
      if (next.plan_version_id !== planDetail?.planVersionId) void loadPlanDetail(next.plan_version_id)
    },
    [planDetail, loadPlanDetail],
  )

  // --- Subscribe (empty state) ---------------------------------------------
  const onSubscribe = async (planVersionId: string, cycle: BillingCycle) => {
    const created = await subscribe(planVersionId, cycle)
    setSub(created)
    await loadPlanDetail(created.plan_version_id)
    toast.success('Subscription started.')
  }

  // --- Lifecycle actions (state-machine gated) ------------------------------
  const runLifecycle = async (action: LifecycleAction) => {
    if (acting) return
    setActing(true)
    try {
      const next =
        action === 'pause'
          ? await pauseSubscription()
          : action === 'resume'
            ? await resumeSubscription()
            : await reactivateSubscription()
      applySub(next)
      toast.success(`${ACTION_LABELS[action]} succeeded.`)
    } catch (err) {
      raise409(err)
    } finally {
      setActing(false)
    }
  }

  const onCancelConfirm = async (immediate: boolean, reason: string) => {
    const next = await cancelSubscription(immediate, reason)
    setSub(next)
    setShowCancel(false)
    toast.success(immediate ? 'Subscription canceled.' : 'Cancellation scheduled for period end.')
  }

  // --- Change plan ----------------------------------------------------------
  const onChangePlan = async (planVersionId: string, cycle: BillingCycle) => {
    const next = await changePlan(planVersionId, cycle)
    applySub(next)
    setShowChangePlan(false)
    toast.success(next.scheduled_change ? 'Plan change scheduled for period end.' : 'Plan changed.')
  }

  const onCancelScheduledChange = async () => {
    if (acting) return
    setActing(true)
    try {
      applySub(await cancelScheduledChange())
      toast.success('Scheduled plan change canceled.')
    } catch (err) {
      raise409(err)
    } finally {
      setActing(false)
    }
  }

  // --- Render ---------------------------------------------------------------
  if (loading) {
    return (
      <>
        <PageBreadcrumb title="Subscription" subtitle="Billing" />
        <p className="text-default-500 py-8 text-center text-sm">Loading subscription…</p>
      </>
    )
  }

  if (error) {
    return (
      <>
        <PageBreadcrumb title="Subscription" subtitle="Billing" />
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
      </>
    )
  }

  // Empty state: no live subscription — offer the subscribe flow.
  if (!sub) {
    return (
      <>
        <PageBreadcrumb title="Subscription" subtitle="Billing" />
        <div className="mb-6 text-center">
          <Icon icon="invoice" className="text-default-400 mx-auto text-4xl" />
          <h3 className="mt-3 text-xl font-bold">No active subscription</h3>
          <p className="text-default-500 mt-1 text-sm">Choose a plan to get started.</p>
        </div>
        <PlanPicker mode="subscribe" onSelect={onSubscribe} />
      </>
    )
  }

  const actions = allowedActions(sub.status)
  const currentPrice = planDetail?.prices.find((p) => p.cycle === sub.billing_cycle) ?? null
  const trialDays = sub.status === 'trialing' ? daysUntil(sub.trial_ends_at) : null

  return (
    <>
      <PageBreadcrumb title="Subscription" subtitle="Billing" />

      <div className="card">
        <div className="card-body">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="mb-2 flex items-center gap-2">
                <h4 className="text-lg font-semibold">{planDetail ? planDetail.planKey : 'Current subscription'}</h4>
                <span className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusChipClass(sub.status)}`}>{STATUS_LABELS[sub.status]}</span>
              </div>
              <p className="text-default-500 text-sm">
                {CYCLE_LABELS[sub.billing_cycle]}
                {currentPrice && planDetail ? ` · ${formatMinor(currentPrice.amount_minor, planDetail.currency)}` : ''}
              </p>
              <p className="text-default-500 mt-1 text-sm">
                Current period: {formatDate(sub.current_period_start)} — {formatDate(sub.current_period_end)}
              </p>
            </div>

            <div className="flex flex-wrap gap-2">
              {actions.map((action) =>
                action === 'cancel' ? (
                  <button
                    key={action}
                    type="button"
                    className="btn btn-sm bg-danger/10 text-danger hover:bg-danger hover:text-white"
                    disabled={acting}
                    onClick={() => setShowCancel(true)}
                  >
                    {ACTION_LABELS.cancel}
                  </button>
                ) : (
                  <button
                    key={action}
                    type="button"
                    className="btn btn-sm bg-light hover:text-primary"
                    disabled={acting}
                    onClick={() => void runLifecycle(action)}
                  >
                    {ACTION_LABELS[action]}
                  </button>
                ),
              )}
              {canModifyPlan(sub.status) && (
                <button type="button" className="btn btn-sm bg-primary text-white hover:bg-primary-hover" onClick={() => setShowChangePlan((v) => !v)}>
                  {showChangePlan ? 'Close' : 'Change plan'}
                </button>
              )}
            </div>
          </div>

          {trialDays !== null && (
            <div className="mt-4 rounded-md bg-info/10 px-4 py-3 text-sm text-info" role="status">
              Trial ends in {trialDays} {trialDays === 1 ? 'day' : 'days'} ({formatDate(sub.trial_ends_at ?? null)}).
            </div>
          )}

          {sub.cancel_at_period_end && (
            <div className="mt-4 rounded-md bg-warning/10 px-4 py-3 text-sm text-warning" role="status">
              This subscription is scheduled to cancel at the end of the current period ({formatDate(sub.current_period_end)}).
            </div>
          )}

          {sub.scheduled_change && (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-md bg-warning/10 px-4 py-3 text-sm text-warning" role="status">
              <span>
                A plan change ({CYCLE_LABELS[sub.scheduled_change.billing_cycle]}) is scheduled for the end of the current period ({formatDate(sub.current_period_end)}).
              </span>
              <button type="button" className="btn btn-sm bg-warning text-white hover:opacity-90" disabled={acting} onClick={() => void onCancelScheduledChange()}>
                Cancel change
              </button>
            </div>
          )}
        </div>
      </div>

      {showChangePlan && (
        <div className="card mt-6">
          <div className="card-header">
            <h4 className="card-title">Change plan</h4>
          </div>
          <div className="card-body">
            <PlanPicker mode="change" currentPlanVersionId={sub.plan_version_id} onSelect={onChangePlan} />
          </div>
        </div>
      )}

      <AddonsSection addons={sub.addons ?? []} planKey={planDetail?.planKey ?? null} editable={canModifyPlan(sub.status)} onChange={applySub} />

      {showCancel && <CancelModal periodEnd={sub.current_period_end} onClose={() => setShowCancel(false)} onConfirm={onCancelConfirm} />}
    </>
  )
}

export default Page
