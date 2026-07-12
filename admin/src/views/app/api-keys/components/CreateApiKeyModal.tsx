// Create-key modal with a one-time secret reveal. The full secret lives only in
// this component's local state; closing the modal unmounts it, so the secret is
// never retained anywhere in the tree after the reveal is dismissed.
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { useState } from 'react'
import { createApiKey, type CreatedApiKey } from '../api'

type Props = {
  onClose: () => void
  /** Called after a successful create so the parent can refresh its list. */
  onCreated: () => void
}

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Could not create the API key. Please try again.'
}

const parseScopes = (raw: string): string[] => raw.split(/\s+/).map((s) => s.trim()).filter(Boolean)

const CreateApiKeyModal = ({ onClose, onCreated }: Props) => {
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [created, setCreated] = useState<CreatedApiKey | null>(null)
  const [copied, setCopied] = useState(false)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError(null)
    try {
      const result = await createApiKey(name.trim(), parseScopes(scopes))
      setCreated(result)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const copySecret = async () => {
    if (!created) return
    try {
      await navigator.clipboard.writeText(created.api_key)
      setCopied(true)
    } catch {
      // Clipboard unavailable — the user can still select the value manually.
    }
  }

  const done = () => {
    onCreated()
    onClose()
  }

  return (
    <div className="fixed inset-0 z-80 flex items-start justify-center overflow-y-auto bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="create-api-key-title">
      <div className="card mt-16 w-full max-w-lg">
        <div className="card-header flex items-center justify-between">
          <h4 id="create-api-key-title" className="card-title">
            {created ? 'API key created' : 'New API key'}
          </h4>
          <button type="button" aria-label="Close" onClick={created ? done : onClose}>
            <Icon icon="x" className="text-xl" />
          </button>
        </div>

        {created ? (
          <div className="card-body">
            <div className="mb-4 flex items-start gap-2 rounded-md bg-warning/10 px-4 py-3 text-sm text-warning">
              <Icon icon="alert-triangle" className="mt-0.5 shrink-0 text-base" />
              <span>Copy this secret now. For security it is shown only once and you won&apos;t be able to see it again.</span>
            </div>
            <label className="form-label" htmlFor="api-key-secret">
              API key secret
            </label>
            <div className="input-group">
              <input id="api-key-secret" aria-label="API key secret" type="text" className="form-input font-mono text-sm" readOnly value={created.api_key} />
              <button type="button" className="btn btn-icon border-default-300" aria-label="Copy secret" onClick={copySecret}>
                <Icon icon={copied ? 'check' : 'copy'} className="text-lg" />
              </button>
            </div>
            {copied && <p className="mt-1 text-xs text-success">Copied to clipboard.</p>}
          </div>
        ) : (
          <form onSubmit={submit}>
            <div className="card-body space-y-4">
              {error && (
                <div className="rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
                  {error}
                </div>
              )}
              <div>
                <label className="form-label" htmlFor="api-key-name">
                  Name
                </label>
                <input id="api-key-name" type="text" className="form-input" placeholder="e.g. Deploy bot" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
              </div>
              <div>
                <label className="form-label" htmlFor="api-key-scopes">
                  Scopes
                </label>
                <input id="api-key-scopes" type="text" className="form-input" placeholder="read:billing write:members" value={scopes} onChange={(e) => setScopes(e.target.value)} />
                <p className="text-default-500 mt-1 text-xs">Space-separated <code>resource:action</code> scopes. Leave blank for no scopes.</p>
              </div>
            </div>
            <div className="card-footer flex items-center justify-end gap-2">
              <button type="button" className="btn bg-light hover:text-primary" onClick={onClose} disabled={submitting}>
                Cancel
              </button>
              <button type="submit" className="btn bg-primary text-white hover:bg-primary-hover" disabled={submitting || name.trim() === ''}>
                {submitting ? 'Creating…' : 'Create key'}
              </button>
            </div>
          </form>
        )}

        {created && (
          <div className="card-footer flex items-center justify-end">
            <button type="button" className="btn bg-primary text-white hover:bg-primary-hover" onClick={done}>
              Done
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

export default CreateApiKeyModal
