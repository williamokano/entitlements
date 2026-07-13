// Add-on version editor. Same draft-only rule as the plan version editor: once
// published, every field is disabled and neither Save nor Publish is offered.
//
// Beyond pricing (integer minor units) it edits: whether quantity > 1 is allowed,
// which plan keys the add-on is compatible with, and the entitlement deltas.
// A delta is either a limit_delta (adds an INTEGER amount to a numeric feature)
// or a value_override (sets the feature to a value). Delta rows are validated —
// non-empty feature key, and a whole-number amount for limit deltas — before any
// PATCH is built, so no float or blank key reaches the API.
import { useCallback, useEffect, useState } from 'react'
import { toast } from 'react-toastify'
import Icon from '@/components/wrappers/Icon'
import { confirmAction } from '@/lib/confirm'
import {
  BILLING_CYCLES,
  getAddonVersion,
  listPlans,
  publishAddonVersion,
  updateAddonVersion,
  type AddonVersionDetail,
  type BillingCycle,
  type Delta,
  type DeltaKind,
  type Price,
} from '../../api'
import { CYCLE_LABELS, errorMessage, formatMinor, parseInteger, parseMinorUnits, versionBadgeClass } from '../../helpers'

type Props = { addonId: string; vid: string }

type CycleRow = { enabled: boolean; amount: string }
type DeltaRow = { id: number; feature_key: string; kind: DeltaKind; amount: string; value: string }

const emptyCycles = (): Record<BillingCycle, CycleRow> => ({
  monthly: { enabled: false, amount: '' },
  annual: { enabled: false, amount: '' },
  custom: { enabled: false, amount: '' },
})

const parseDeltaValue = (raw: string): unknown => {
  const t = raw.trim()
  if (t === 'true') return true
  if (t === 'false') return false
  if (/^-?\d+$/.test(t)) return Number(t)
  return raw
}

const stringifyValue = (value: unknown): string => {
  if (value === undefined || value === null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'boolean' || typeof value === 'number') return String(value)
  return JSON.stringify(value)
}

const AddonVersionEditor = ({ addonId, vid }: Props) => {
  const [version, setVersion] = useState<AddonVersionDetail | null>(null)
  const [planKeys, setPlanKeys] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [publishing, setPublishing] = useState(false)

  const [currency, setCurrency] = useState('')
  const [cycles, setCycles] = useState<Record<BillingCycle, CycleRow>>(emptyCycles)
  const [quantityAllowed, setQuantityAllowed] = useState(false)
  const [compatible, setCompatible] = useState<string[]>([])
  const [deltas, setDeltas] = useState<DeltaRow[]>([])
  const [nextDeltaId, setNextDeltaId] = useState(1)

  const hydrate = useCallback((v: AddonVersionDetail) => {
    setVersion(v)
    setCurrency(v.currency ?? '')
    const rows = emptyCycles()
    for (const p of v.prices ?? []) {
      rows[p.cycle] = { enabled: true, amount: String(p.amount_minor) }
    }
    setCycles(rows)
    setQuantityAllowed(v.quantity_allowed ?? false)
    setCompatible(v.compatible_plan_keys ?? [])
    const deltaRows = (v.deltas ?? []).map((d, i) => ({
      id: i + 1,
      feature_key: d.feature_key,
      kind: d.kind,
      amount: d.kind === 'limit_delta' && d.amount !== undefined ? String(d.amount) : '',
      value: d.kind === 'value_override' ? stringifyValue(d.value) : '',
    }))
    setDeltas(deltaRows)
    setNextDeltaId(deltaRows.length + 1)
  }, [])

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const [v, plans] = await Promise.all([getAddonVersion(vid, signal), listPlans(signal).catch(() => [])])
        if (signal?.aborted) return
        hydrate(v)
        setPlanKeys(plans.map((p) => p.key))
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

  const toggleCompatible = (key: string) =>
    setCompatible((prev) => (prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]))

  const addDelta = () => {
    setDeltas((prev) => [...prev, { id: nextDeltaId, feature_key: '', kind: 'limit_delta', amount: '', value: '' }])
    setNextDeltaId((n) => n + 1)
  }
  const removeDelta = (id: number) => setDeltas((prev) => prev.filter((d) => d.id !== id))
  const setDelta = (id: number, patch: Partial<DeltaRow>) =>
    setDeltas((prev) => prev.map((d) => (d.id === id ? { ...d, ...patch } : d)))

  const buildContent = (): { error: string } | { content: Parameters<typeof updateAddonVersion>[2] } => {
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

    const built: Delta[] = []
    const seen = new Set<string>()
    for (const d of deltas) {
      const key = d.feature_key.trim()
      if (key === '') return { error: 'Every delta needs a feature key.' }
      if (seen.has(key)) return { error: `Duplicate delta for feature "${key}".` }
      seen.add(key)
      if (d.kind === 'limit_delta') {
        const amount = parseInteger(d.amount)
        if (amount === null) return { error: `Delta "${key}" amount must be a whole number (integer minor units / count).` }
        built.push({ feature_key: key, kind: 'limit_delta', amount })
      } else {
        if (d.value.trim() === '') return { error: `Delta "${key}" needs a value for the override.` }
        built.push({ feature_key: key, kind: 'value_override', value: parseDeltaValue(d.value) })
      }
    }

    return {
      content: { currency: code, prices, quantity_allowed: quantityAllowed, compatible_plan_keys: compatible, deltas: built },
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
      const updated = await updateAddonVersion(addonId, vid, built.content)
      hydrate({ ...updated, addon_key: version?.addon_key ?? '' })
      toast.success('Draft version saved.')
    } catch (err) {
      setSaveError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const onPublish = async () => {
    if (publishing || readOnly) return
    const built = buildContent()
    if ('error' in built) {
      setSaveError(built.error)
      return
    }
    const ok = await confirmAction({
      title: 'Publish this version?',
      text: 'Publishing freezes this add-on version permanently. Its pricing, compatibility, and entitlement deltas become immutable — future changes require a new version. This cannot be undone.',
      confirmText: 'Publish',
    })
    if (!ok) return
    setSaveError(null)
    setPublishing(true)
    try {
      await updateAddonVersion(addonId, vid, built.content)
      const published = await publishAddonVersion(addonId, vid)
      hydrate({ ...published, addon_key: version?.addon_key ?? '' })
      toast.success('Version published.')
    } catch (err) {
      setSaveError(errorMessage(err))
    } finally {
      setPublishing(false)
    }
  }

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
          <h4 className="text-lg font-semibold">Version {version.version}</h4>
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
          <span>This version is published and immutable. Create a new version to change pricing, compatibility, or deltas.</span>
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
              <label className="form-label" htmlFor="addon-currency">
                Currency&nbsp;<span className="text-danger">*</span>
              </label>
              <input
                id="addon-currency"
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
            <label className="flex items-center gap-2">
              <input type="checkbox" className="form-checkbox" checked={quantityAllowed} onChange={(e) => setQuantityAllowed(e.target.checked)} />
              <span className="text-sm font-medium">Allow quantity greater than 1</span>
            </label>
          </div>
        </div>

        <div className="card">
          <div className="card-header">
            <h5 className="card-title">Compatible plans</h5>
          </div>
          <div className="card-body">
            {planKeys.length === 0 ? (
              <p className="text-default-500 py-2 text-sm">No plans available to select.</p>
            ) : (
              <div className="flex flex-wrap gap-3">
                {planKeys.map((key) => (
                  <label key={key} className="flex items-center gap-2 rounded-md border border-default-200 px-3 py-2">
                    <input type="checkbox" className="form-checkbox" checked={compatible.includes(key)} disabled={readOnly} onChange={() => toggleCompatible(key)} />
                    <code className="text-sm">{key}</code>
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h5 className="card-title">Entitlement deltas</h5>
            {!readOnly && (
              <button type="button" className="btn btn-sm border border-default-300 hover:border-primary hover:text-primary" onClick={addDelta}>
                <Icon icon="plus" className="text-base" />
                Add delta
              </button>
            )}
          </div>
          <div className="card-body">
            {deltas.length === 0 ? (
              <p className="text-default-500 py-4 text-center text-sm">No deltas. A limit delta adds a whole-number amount; an override sets a value.</p>
            ) : (
              <div className="space-y-3">
                {deltas.map((d) => (
                  <div key={d.id} className="flex flex-wrap items-center gap-3">
                    <input
                      type="text"
                      aria-label="Delta feature key"
                      className="form-input flex-1 font-mono"
                      placeholder="feature.key"
                      value={d.feature_key}
                      disabled={readOnly}
                      onChange={(e) => setDelta(d.id, { feature_key: e.target.value })}
                    />
                    <select
                      aria-label="Delta kind"
                      className="form-select w-44"
                      value={d.kind}
                      disabled={readOnly}
                      onChange={(e) => setDelta(d.id, { kind: e.target.value as DeltaKind })}
                    >
                      <option value="limit_delta">Limit delta</option>
                      <option value="value_override">Value override</option>
                    </select>
                    {d.kind === 'limit_delta' ? (
                      <input
                        type="text"
                        inputMode="numeric"
                        aria-label="Delta amount"
                        className="form-input w-40"
                        placeholder="integer amount"
                        value={d.amount}
                        disabled={readOnly}
                        onChange={(e) => setDelta(d.id, { amount: e.target.value })}
                      />
                    ) : (
                      <input
                        type="text"
                        aria-label="Delta value"
                        className="form-input w-40"
                        placeholder="value"
                        value={d.value}
                        disabled={readOnly}
                        onChange={(e) => setDelta(d.id, { value: e.target.value })}
                      />
                    )}
                    {!readOnly && (
                      <button
                        type="button"
                        className="btn btn-icon btn-sm border border-default-300 hover:border-danger hover:text-danger"
                        aria-label={`Remove delta ${d.feature_key || '(unnamed)'}`}
                        onClick={() => removeDelta(d.id)}
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

export default AddonVersionEditor
