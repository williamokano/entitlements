// Addons section: the subscription's attached addons with quantity steppers,
// an attach form fed by the catalog's compatible published addon versions, and
// a detach action guarded by a confirm. Quantity changes re-POST /addons (the
// backend upserts on (subscription, addon_version)); a 400 (incompatible addon
// or bad quantity) renders inline in the attach form.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { toast } from 'react-toastify'
import Icon from '@/components/wrappers/Icon'
import { confirmAction } from '@/lib/confirm'
import { getAddon, listAddons, type Price } from '../../catalog/api'
import { errorMessage, formatMinor, parseInteger, priceSummary } from '../../catalog/helpers'
import { attachAddon, detachAddon, type Subscription, type SubscriptionAddon } from '../api'

// A published addon version flattened with its parent addon's naming, keyed for
// resolving attached rows and populating the compatible-to-attach picker.
type CatalogAddonVersion = {
  id: string
  addonName: string
  addonKey: string
  version: number
  currency: string
  prices: Price[]
  quantityAllowed: boolean
  compatiblePlanKeys: string[]
}

type Props = {
  addons: SubscriptionAddon[]
  /** The current plan's key — compatibility is filtered against it. */
  planKey: string | null
  /** Whether edits are offered (hidden once the subscription is terminal). */
  editable: boolean
  /** Called with the fresh subscription after any successful mutation. */
  onChange: (sub: Subscription) => void
}

const AddonsSection = ({ addons, planKey, editable, onChange }: Props) => {
  const [catalog, setCatalog] = useState<CatalogAddonVersion[]>([])
  const [loadingCatalog, setLoadingCatalog] = useState(true)
  const [busyId, setBusyId] = useState<string | null>(null)

  // Attach-form state.
  const [selectedId, setSelectedId] = useState('')
  const [quantity, setQuantity] = useState('1')
  const [attaching, setAttaching] = useState(false)
  const [attachError, setAttachError] = useState<string | null>(null)

  const loadCatalog = useCallback(async (signal?: AbortSignal) => {
    try {
      const list = await listAddons(signal)
      const details = await Promise.all(list.map((a) => getAddon(a.id, signal)))
      if (signal?.aborted) return
      const flat = details.flatMap(({ addon, versions }) =>
        versions
          .filter((v) => v.status === 'published')
          .map((v) => ({
            id: v.id,
            addonName: addon.name,
            addonKey: addon.key,
            version: v.version,
            currency: v.currency,
            prices: v.prices,
            quantityAllowed: v.quantity_allowed,
            compatiblePlanKeys: v.compatible_plan_keys,
          })),
      )
      setCatalog(flat)
    } catch {
      // The catalog is a convenience for names + the attach picker; if it can't
      // be read the attached addons still render (by id) and the section degrades
      // gracefully rather than blocking the whole page.
      if (!signal?.aborted) setCatalog([])
    } finally {
      if (!signal?.aborted) setLoadingCatalog(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await loadCatalog(controller.signal)
    })()
    return () => controller.abort()
  }, [loadCatalog])

  const byId = useMemo(() => new Map(catalog.map((v) => [v.id, v])), [catalog])
  const attachedIds = useMemo(() => new Set(addons.map((a) => a.addon_version_id)), [addons])

  // Only published versions compatible with the current plan, not already on.
  const attachable = useMemo(
    () => catalog.filter((v) => planKey !== null && v.compatiblePlanKeys.includes(planKey) && !attachedIds.has(v.id)),
    [catalog, planKey, attachedIds],
  )

  const setQuantityFor = async (addon: SubscriptionAddon, next: number) => {
    if (next < 1 || busyId) return
    setBusyId(addon.addon_version_id)
    try {
      onChange(await attachAddon(addon.addon_version_id, next))
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  const detach = async (addon: SubscriptionAddon) => {
    const meta = byId.get(addon.addon_version_id)
    const confirmed = await confirmAction({
      title: 'Detach addon',
      text: `Remove ${meta ? meta.addonName : 'this addon'} from the subscription?`,
      confirmText: 'Detach',
    })
    if (!confirmed) return
    setBusyId(addon.addon_version_id)
    try {
      onChange(await detachAddon(addon.addon_version_id))
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  const attach = async (event: React.FormEvent) => {
    event.preventDefault()
    if (attaching) return
    setAttachError(null)
    if (selectedId === '') {
      setAttachError('Select an addon to attach.')
      return
    }
    const meta = byId.get(selectedId)
    const qty = meta && !meta.quantityAllowed ? 1 : parseInteger(quantity)
    if (qty === null || qty < 1) {
      setAttachError('Quantity must be a whole number of at least 1.')
      return
    }
    setAttaching(true)
    try {
      onChange(await attachAddon(selectedId, qty))
      setSelectedId('')
      setQuantity('1')
    } catch (err) {
      // A 400 (incompatible addon / invalid quantity) surfaces inline here.
      setAttachError(errorMessage(err))
    } finally {
      setAttaching(false)
    }
  }

  const selectedMeta = selectedId ? byId.get(selectedId) : undefined
  const quantityDisabled = !!selectedMeta && !selectedMeta.quantityAllowed

  return (
    <div className="card mt-6">
      <div className="card-header">
        <h4 className="card-title">Addons</h4>
      </div>
      <div className="card-body space-y-6">
        {addons.length === 0 ? (
          <p className="text-default-500 text-sm">No addons attached.</p>
        ) : (
          <ul className="divide-default-200 divide-y">
            {addons.map((addon) => {
              const meta = byId.get(addon.addon_version_id)
              const busy = busyId === addon.addon_version_id
              return (
                <li key={addon.addon_version_id} className="flex items-center justify-between py-3">
                  <div>
                    <p className="font-medium">{meta ? meta.addonName : addon.addon_version_id}</p>
                    <p className="text-default-500 text-sm">
                      {meta ? (
                        <>
                          <code className="text-xs">{meta.addonKey}</code> · v{meta.version} · {priceSummary(meta.prices, meta.currency)}
                        </>
                      ) : (
                        'Addon details unavailable'
                      )}
                    </p>
                  </div>
                  <div className="flex items-center gap-3">
                    {meta && !meta.quantityAllowed ? (
                      <span className="text-default-500 text-sm">Qty 1</span>
                    ) : (
                      <div className="flex items-center gap-2" aria-label={`Quantity for ${meta ? meta.addonName : 'addon'}`}>
                        <button
                          type="button"
                          className="btn btn-icon btn-sm bg-light hover:text-primary"
                          aria-label="Decrease quantity"
                          disabled={!editable || busy || addon.quantity <= 1}
                          onClick={() => void setQuantityFor(addon, addon.quantity - 1)}
                        >
                          <Icon icon="minus" />
                        </button>
                        <span className="w-8 text-center font-medium" aria-label="Quantity">
                          {addon.quantity}
                        </span>
                        <button
                          type="button"
                          className="btn btn-icon btn-sm bg-light hover:text-primary"
                          aria-label="Increase quantity"
                          disabled={!editable || busy}
                          onClick={() => void setQuantityFor(addon, addon.quantity + 1)}
                        >
                          <Icon icon="plus" />
                        </button>
                      </div>
                    )}
                    {editable && (
                      <button
                        type="button"
                        className="btn btn-icon btn-sm bg-danger/10 text-danger hover:bg-danger hover:text-white"
                        aria-label={`Detach ${meta ? meta.addonName : 'addon'}`}
                        disabled={busy}
                        onClick={() => void detach(addon)}
                      >
                        <Icon icon="trash" />
                      </button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        )}

        {editable && (
          <form onSubmit={attach} className="border-default-200 border-t pt-4">
            <h5 className="mb-3 font-medium">Attach an addon</h5>
            {attachError && (
              <div className="mb-3 rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
                {attachError}
              </div>
            )}
            {loadingCatalog ? (
              <p className="text-default-500 text-sm">Loading compatible addons…</p>
            ) : attachable.length === 0 ? (
              <p className="text-default-500 text-sm">No compatible published addons available to attach.</p>
            ) : (
              <div className="flex flex-wrap items-end gap-3">
                <div className="min-w-56 flex-1">
                  <label className="form-label" htmlFor="addon-select">
                    Addon
                  </label>
                  <select id="addon-select" className="form-select" value={selectedId} onChange={(e) => setSelectedId(e.target.value)}>
                    <option value="">Select an addon…</option>
                    {attachable.map((v) => (
                      <option key={v.id} value={v.id}>
                        {v.addonName} — {formatMinor(v.prices[0]?.amount_minor ?? 0, v.currency)}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="w-28">
                  <label className="form-label" htmlFor="addon-qty">
                    Quantity
                  </label>
                  <input
                    id="addon-qty"
                    type="number"
                    min={1}
                    className="form-input"
                    value={quantityDisabled ? 1 : quantity}
                    disabled={quantityDisabled}
                    onChange={(e) => setQuantity(e.target.value)}
                  />
                </div>
                <button type="submit" className="btn bg-primary text-white hover:bg-primary-hover" disabled={attaching}>
                  {attaching ? 'Attaching…' : 'Attach'}
                </button>
              </div>
            )}
          </form>
        )}
      </div>
    </div>
  )
}

export default AddonsSection
