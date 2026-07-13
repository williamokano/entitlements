// Tenant settings: loads the current tenant, edits name + settings (JSON), and
// PATCHes the round-trip with a success toast. The slug is fixed at creation
// (the backend PATCH does not accept a slug change), so it is shown read-only.
import { useCallback, useEffect, useState } from 'react'
import { toast } from 'react-toastify'
import { ApiError } from '@/lib/api'
import { rememberTenant } from '@/lib/tenant'
import { cn } from '@/utils/helpers'
import { getTenant, updateTenant, type Tenant } from '../api'

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

const formatSettings = (settings: Record<string, unknown> | undefined): string =>
  settings && Object.keys(settings).length > 0 ? JSON.stringify(settings, null, 2) : '{}'

type Props = {
  tenantId: string
  /** Notified whenever the loaded/updated tenant changes (keeps the page in sync). */
  onTenantChange?: (tenant: Tenant) => void
}

const TenantSettingsForm = ({ tenantId, onTenantChange }: Props) => {
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [name, setName] = useState('')
  const [settingsText, setSettingsText] = useState('{}')
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const applyTenant = useCallback(
    (t: Tenant) => {
      setTenant(t)
      setName(t.name)
      setSettingsText(formatSettings(t.settings))
      rememberTenant({ id: t.id, name: t.name, slug: t.slug })
      onTenantChange?.(t)
    },
    [onTenantChange],
  )

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const t = await getTenant(tenantId, signal)
        if (signal?.aborted) return
        applyTenant(t)
        setLoadError(null)
      } catch (err) {
        if (signal?.aborted) return
        setLoadError(errorMessage(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [tenantId, applyTenant],
  )

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (saving) return
    setSaveError(null)
    setSettingsError(null)

    let settings: Record<string, unknown>
    try {
      const parsed = JSON.parse(settingsText || '{}')
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        throw new Error('not an object')
      }
      settings = parsed as Record<string, unknown>
    } catch {
      setSettingsError('Settings must be a valid JSON object.')
      return
    }

    setSaving(true)
    try {
      const updated = await updateTenant(tenantId, { name: name.trim(), settings })
      // The backend response does not echo settings; preserve what we sent.
      applyTenant({ ...updated, settings })
      toast.success('Tenant settings saved.')
    } catch (err) {
      setSaveError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="card">
        <div className="card-body">
          <p className="text-default-500 py-8 text-center text-sm">Loading tenant…</p>
        </div>
      </div>
    )
  }

  if (loadError && !tenant) {
    return (
      <div className="card">
        <div className="card-body">
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
        </div>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-header">
        <h4 className="card-title">General</h4>
      </div>
      <div className="card-body">
        <form onSubmit={onSubmit} noValidate>
          {saveError && (
            <div className="mb-4 rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
              {saveError}
            </div>
          )}

          <div className="mb-5">
            <label htmlFor="tenant-name" className="form-label">
              Tenant name&nbsp;<span className="text-danger">*</span>
            </label>
            <input id="tenant-name" type="text" className="form-input" value={name} onChange={(e) => setName(e.target.value)} required />
          </div>

          <div className="mb-5">
            <label htmlFor="tenant-slug" className="form-label">
              Slug
            </label>
            <input id="tenant-slug" type="text" className="form-input bg-default-100" value={tenant?.slug ?? ''} readOnly disabled />
            <p className="text-default-400 mt-1 text-xs">The slug is fixed once the tenant is created.</p>
          </div>

          <div className="mb-6">
            <label htmlFor="tenant-settings" className="form-label">
              Settings (JSON)
            </label>
            <textarea
              id="tenant-settings"
              rows={6}
              className={cn('form-input font-mono text-sm', settingsError && '!border-danger')}
              value={settingsText}
              onChange={(e) => setSettingsText(e.target.value)}
              spellCheck={false}
            />
            {settingsError && <p className="text-danger mt-1 text-sm">{settingsError}</p>}
          </div>

          <button type="submit" disabled={saving || name.trim() === ''} className="btn bg-primary font-semibold text-white hover:bg-primary-hover">
            {saving ? 'Saving…' : 'Save changes'}
          </button>
        </form>
      </div>
    </div>
  )
}

export default TenantSettingsForm
