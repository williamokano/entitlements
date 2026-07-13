// Plan version editor. Editable ONLY while the version is a draft: once
// published, every field is disabled and neither Save nor Publish is offered —
// a published version is immutable, so edits happen by cutting a new version.
//
// Money discipline: pricing is entered and kept as integer minor units. The
// amount inputs accept digits only, parseMinorUnits rejects anything non-integer
// before a PATCH is built, and the major-unit figure is shown read-only for
// orientation. No float ever enters the model.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { toast } from 'react-toastify'
import Icon from '@/components/wrappers/Icon'
import { confirmAction } from '@/lib/confirm'
import {
  BILLING_CYCLES,
  getPlanVersion,
  publishPlanVersion,
  updatePlanVersion,
  type BillingCycle,
  type PlanVersionDetail,
  type Price,
} from '../../api'
import { CYCLE_LABELS, errorMessage, formatMinor, parseInteger, parseMinorUnits, versionBadgeClass } from '../../helpers'

type Props = { planId: string; vid: string }

type CycleRow = { enabled: boolean; amount: string }
type GrantRow = { id: number; key: string; value: string }

const emptyCycles = (): Record<BillingCycle, CycleRow> => ({
  monthly: { enabled: false, amount: '' },
  annual: { enabled: false, amount: '' },
  custom: { enabled: false, amount: '' },
})

// Feature-grant values round-trip as text: "true"/"false" -> boolean, a whole
// number -> integer, otherwise the raw string. Limits stay integers (no floats).
const parseGrantValue = (raw: string): unknown => {
  const t = raw.trim()
  if (t === 'true') return true
  if (t === 'false') return false
  if (/^-?\d+$/.test(t)) return Number(t)
  return raw
}

const stringifyGrantValue = (value: unknown): string => {
  if (typeof value === 'string') return value
  if (typeof value === 'boolean' || typeof value === 'number') return String(value)
  return JSON.stringify(value)
}

const VersionEditor = ({ planId, vid }: Props) => {
  const [version, setVersion] = useState<PlanVersionDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [publishing, setPublishing] = useState(false)

  const [currency, setCurrency] = useState('')
  const [cycles, setCycles] = useState<Record<BillingCycle, CycleRow>>(emptyCycles)
  const [trialEnabled, setTrialEnabled] = useState(false)
  const [trialDays, setTrialDays] = useState('')
  const [cardRequired, setCardRequired] = useState(false)
  const [graceDays, setGraceDays] = useState('0')
  const [grants, setGrants] = useState<GrantRow[]>([])
  const [nextGrantId, setNextGrantId] = useState(1)

  const hydrate = useCallback((v: PlanVersionDetail) => {
    setVersion(v)
    setCurrency(v.currency ?? '')
    const rows = emptyCycles()
    for (const p of v.prices ?? []) {
      rows[p.cycle] = { enabled: true, amount: String(p.amount_minor) }
    }
    setCycles(rows)
    setTrialEnabled(v.trial?.enabled ?? false)
    setTrialDays(v.trial?.days ? String(v.trial.days) : '')
    setCardRequired(v.trial?.card_required ?? false)
    setGraceDays(String(v.grace_days ?? 0))
    const grantRows = Object.entries(v.feature_grants ?? {}).map(([key, value], i) => ({
      id: i + 1,
      key,
      value: stringifyGrantValue(value),
    }))
    setGrants(grantRows)
    setNextGrantId(grantRows.length + 1)
  }, [])

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const v = await getPlanVersion(vid, signal)
        if (signal?.aborted) return
        hydrate(v)
        setLoadError(null)
      } catch (err) {
        if (signal?.aborted) return
        setLoadError(errorMessage(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [vid, hydrate],
  )

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  const readOnly = version?.status === 'published'

  const setCycle = (cycle: BillingCycle, patch: Partial<CycleRow>) =>
    setCycles((prev) => ({ ...prev, [cycle]: { ...prev[cycle], ...patch } }))

  const majorPreview = (cycle: BillingCycle): string => {
    const minor = parseMinorUnits(cycles[cycle].amount)
    if (minor === null || !currency) return ''
    return formatMinor(minor, currency)
  }

  // Build the PATCH body, validating minor-units + integers up front. Returns a
  // string error to display, or the content to send.
  const buildContent = (): { error: string } | { content: Parameters<typeof updatePlanVersion>[2] } => {
    const code = currency.trim().toUpperCase()
    if (!/^[A-Z]{3}$/.test(code)) return { error: 'Currency must be a 3-letter ISO code (e.g. USD).' }

    const prices: Price[] = []
    for (const cycle of BILLING_CYCLES) {
      const row = cycles[cycle]
      if (!row.enabled) continue
      const minor = parseMinorUnits(row.amount)
      if (minor === null) return { error: `${CYCLE_LABELS[cycle]} price must be a whole number of minor units.` }
      if (minor < 0) return { error: `${CYCLE_LABELS[cycle]} price must not be negative.` }
      prices.push({ cycle, amount_minor: minor })
    }
    if (prices.length === 0) return { error: 'Enable at least one billing cycle with a price.' }

    const grace = parseInteger(graceDays)
    if (grace === null || grace < 0) return { error: 'Grace days must be a non-negative whole number.' }

    let days = 0
    if (trialEnabled) {
      const parsed = parseInteger(trialDays)
      if (parsed === null || parsed <= 0) return { error: 'Trial days must be a positive whole number when the trial is enabled.' }
      days = parsed
    }

    const featureGrants: Record<string, unknown> = {}
    for (const g of grants) {
      const key = g.key.trim()
      if (key === '') return { error: 'Feature grant keys must not be empty.' }
      if (key in featureGrants) return { error: `Duplicate feature grant "${key}".` }
      featureGrants[key] = parseGrantValue(g.value)
    }

    return {
      content: {
        currency: code,
        prices,
        trial: { enabled: trialEnabled, days, card_required: trialEnabled ? cardRequired : false },
        grace_days: grace,
        feature_grants: featureGrants,
      },
    }
  }

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (saving || readOnly) return
    const built = buildContent()
    if ('error' in built) {
      setSaveError(built.error)
      return
    }
    setSaveError(null)
    setSaving(true)
    try {
      const updated = await updatePlanVersion(planId, vid, built.content)
      hydrate({ ...updated, plan_key: version?.plan_key ?? '' })
      toast.success('Draft version saved.')
    } catch (err) {
      setSaveError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const onPublish = async () => {
    if (publishing || readOnly) return
    // Save any pending edits first so publish freezes exactly what is on screen.
    const built = buildContent()
    if ('error' in built) {
      setSaveError(built.error)
      return
    }
    const ok = await confirmAction({
      title: 'Publish this version?',
      text: 'Publishing freezes this version permanently. Its pricing, trial, grace, and feature grants become immutable — future changes require a new version. This cannot be undone.',
      confirmText: 'Publish',
    })
    if (!ok) return
    setSaveError(null)
    setPublishing(true)
    try {
      await updatePlanVersion(planId, vid, built.content)
      const published = await publishPlanVersion(planId, vid)
      hydrate({ ...published, plan_key: version?.plan_key ?? '' })
      toast.success('Version published.')
    } catch (err) {
      setSaveError(errorMessage(err))
    } finally {
      setPublishing(false)
    }
  }

  const addGrant = () => {
    setGrants((prev) => [...prev, { id: nextGrantId, key: '', value: '' }])
    setNextGrantId((n) => n + 1)
  }
  const removeGrant = (id: number) => setGrants((prev) => prev.filter((g) => g.id !== id))
  const setGrant = (id: number, patch: Partial<GrantRow>) =>
    setGrants((prev) => prev.map((g) => (g.id === id ? { ...g, ...patch } : g)))

  const heading = useMemo(() => (version ? `Version ${version.version}` : 'Version'), [version])

  if (loading) return <p className="text-default-500 py-8 text-center text-sm">Loading version…</p>

  if (loadError && !version) {
    return (
      <div className="flex items-center justify-between rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
        <span>{loadError}</span>
        <button
          type="button"
          className="btn btn-sm bg-danger text-white hover:bg-danger-hover"
          onClick={() => {
            setLoading(true)
            setLoadError(null)
            void load()
          }}
        >
          Retry
        </button>
      </div>
    )
  }

  if (!version) return null

  return (
    <form onSubmit={onSubmit} noValidate>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h4 className="text-lg font-semibold">{heading}</h4>
          <span className={`badge badge-label ${versionBadgeClass(version.status)}`}>{version.status}</span>
        </div>
        {!readOnly && (
          <div className="flex items-center gap-2">
            <button type="submit" className="btn btn-sm bg-primary text-white hover:bg-primary-hover" disabled={saving || publishing}>
              {saving ? 'Saving…' : 'Save draft'}
            </button>
            <button type="button" className="btn btn-sm bg-success text-white hover:bg-success-hover" disabled={saving || publishing} onClick={() => void onPublish()}>
              {publishing ? 'Publishing…' : 'Publish'}
            </button>
          </div>
        )}
      </div>

      {readOnly && (
        <div className="mb-4 flex items-start gap-2 rounded-md bg-info/10 px-4 py-3 text-sm text-info" role="status">
          <Icon icon="lock" className="mt-0.5 shrink-0 text-base" />
          <span>This version is published and immutable. Create a new version to change pricing or features.</span>
        </div>
      )}

      {saveError && (
        <div className="mb-4 rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          {saveError}
        </div>
      )}

      <fieldset disabled={readOnly} className="space-y-6">
        <div className="card">
          <div className="card-header">
            <h5 className="card-title">Pricing</h5>
          </div>
          <div className="card-body space-y-4">
            <div className="max-w-xs">
              <label className="form-label" htmlFor="version-currency">
                Currency&nbsp;<span className="text-danger">*</span>
              </label>
              <input
                id="version-currency"
                type="text"
                className="form-input font-mono uppercase"
                maxLength={3}
                placeholder="USD"
                value={currency}
                onChange={(e) => setCurrency(e.target.value.toUpperCase())}
              />
            </div>
            <div className="space-y-3">
              <p className="text-default-500 text-sm">Amounts are integer minor units (e.g. 1999 = 19.99). No fractional values.</p>
              {BILLING_CYCLES.map((cycle) => (
                <div key={cycle} className="flex flex-wrap items-center gap-3">
                  <label className="flex w-32 items-center gap-2">
                    <input type="checkbox" className="form-checkbox" checked={cycles[cycle].enabled} onChange={(e) => setCycle(cycle, { enabled: e.target.checked })} />
                    <span className="text-sm font-medium">{CYCLE_LABELS[cycle]}</span>
                  </label>
                  <input
                    type="text"
                    inputMode="numeric"
                    aria-label={`${CYCLE_LABELS[cycle]} price (minor units)`}
                    className="form-input w-40"
                    placeholder="minor units"
                    value={cycles[cycle].amount}
                    disabled={readOnly || !cycles[cycle].enabled}
                    onChange={(e) => setCycle(cycle, { amount: e.target.value })}
                  />
                  {majorPreview(cycle) && <span className="text-default-500 text-sm">= {majorPreview(cycle)}</span>}
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="card">
          <div className="card-header">
            <h5 className="card-title">Trial &amp; grace</h5>
          </div>
          <div className="card-body space-y-4">
            <label className="flex items-center gap-2">
              <input type="checkbox" className="form-checkbox" checked={trialEnabled} onChange={(e) => setTrialEnabled(e.target.checked)} />
              <span className="text-sm font-medium">Offer a trial</span>
            </label>
            {trialEnabled && (
              <div className="flex flex-wrap items-center gap-6 ps-6">
                <div>
                  <label className="form-label" htmlFor="trial-days">
                    Trial days
                  </label>
                  <input id="trial-days" type="text" inputMode="numeric" className="form-input w-32" value={trialDays} disabled={readOnly} onChange={(e) => setTrialDays(e.target.value)} />
                </div>
                <label className="flex items-center gap-2 pt-6">
                  <input type="checkbox" className="form-checkbox" checked={cardRequired} disabled={readOnly} onChange={(e) => setCardRequired(e.target.checked)} />
                  <span className="text-sm font-medium">Card required</span>
                </label>
              </div>
            )}
            <div className="max-w-xs">
              <label className="form-label" htmlFor="grace-days">
                Grace days
              </label>
              <input id="grace-days" type="text" inputMode="numeric" className="form-input" value={graceDays} onChange={(e) => setGraceDays(e.target.value)} />
              <p className="text-default-400 mt-1 text-xs">Days a past-due subscription keeps access before losing entitlements.</p>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h5 className="card-title">Feature grants &amp; limits</h5>
            {!readOnly && (
              <button type="button" className="btn btn-sm border border-default-300 hover:border-primary hover:text-primary" onClick={addGrant}>
                <Icon icon="plus" className="text-base" />
                Add grant
              </button>
            )}
          </div>
          <div className="card-body">
            {grants.length === 0 ? (
              <p className="text-default-500 py-4 text-center text-sm">No feature grants. Add key→value entries (numbers stay integers, true/false for flags).</p>
            ) : (
              <div className="space-y-3">
                {grants.map((g) => (
                  <div key={g.id} className="flex flex-wrap items-center gap-3">
                    <input
                      type="text"
                      aria-label="Feature key"
                      className="form-input flex-1 font-mono"
                      placeholder="feature.key"
                      value={g.key}
                      disabled={readOnly}
                      onChange={(e) => setGrant(g.id, { key: e.target.value })}
                    />
                    <input
                      type="text"
                      aria-label="Feature value"
                      className="form-input flex-1"
                      placeholder="value (e.g. 100, true, tier-a)"
                      value={g.value}
                      disabled={readOnly}
                      onChange={(e) => setGrant(g.id, { value: e.target.value })}
                    />
                    {!readOnly && (
                      <button
                        type="button"
                        className="btn btn-icon btn-sm border border-default-300 hover:border-danger hover:text-danger"
                        aria-label={`Remove grant ${g.key || '(unnamed)'}`}
                        onClick={() => removeGrant(g.id)}
                      >
                        <Icon icon="trash" className="text-base" />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </fieldset>
    </form>
  )
}

export default VersionEditor
