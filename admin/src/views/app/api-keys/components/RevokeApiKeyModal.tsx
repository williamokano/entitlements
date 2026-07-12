// Revoke confirmation. Revoking is irreversible, so we require an explicit
// confirm before calling DELETE; on success the parent drops the row.
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { useState } from 'react'
import { revokeApiKey, type ApiKey } from '../api'

type Props = {
  apiKey: ApiKey
  onClose: () => void
  onRevoked: (id: string) => void
}

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Could not revoke the API key. Please try again.'
}

const RevokeApiKeyModal = ({ apiKey, onClose, onRevoked }: Props) => {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const confirm = async () => {
    if (submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await revokeApiKey(apiKey.id)
      onRevoked(apiKey.id)
    } catch (err) {
      setError(errorMessage(err))
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-80 flex items-start justify-center overflow-y-auto bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="revoke-api-key-title">
      <div className="card mt-24 w-full max-w-md">
        <div className="card-header flex items-center justify-between">
          <h4 id="revoke-api-key-title" className="card-title">
            Revoke API key
          </h4>
          <button type="button" aria-label="Close" onClick={onClose} disabled={submitting}>
            <Icon icon="x" className="text-xl" />
          </button>
        </div>
        <div className="card-body">
          {error && (
            <div className="mb-4 rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
              {error}
            </div>
          )}
          <p className="text-sm">
            Revoke <span className="font-semibold">{apiKey.name}</span> (<code className="text-xs">{apiKey.prefix}</code>)? Any client using this key will immediately lose
            access. This cannot be undone.
          </p>
        </div>
        <div className="card-footer flex items-center justify-end gap-2">
          <button type="button" className="btn bg-light hover:text-primary" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="button" className="btn bg-danger text-white hover:bg-danger-hover" onClick={confirm} disabled={submitting}>
            {submitting ? 'Revoking…' : 'Revoke'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default RevokeApiKeyModal
