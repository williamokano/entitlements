// Create-item modal shared by plans and add-ons: both are created from a
// { key, name } pair and return the new entity plus its first draft version.
// The key must be lowercase alphanumerics and single hyphens (mirrors the
// backend's validation) so we surface a clear inline hint before submitting.
import { useState } from 'react'
import Icon from '@/components/wrappers/Icon'
import { errorMessage } from '../helpers'

const KEY_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/

type Props = {
  /** "Plan" or "Add-on" — used in the title and the success routing message. */
  noun: string
  onClose: () => void
  onCreate: (key: string, name: string) => Promise<void>
}

const CreateItemModal = ({ noun, onClose, onCreate }: Props) => {
  const [key, setKey] = useState('')
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const trimmedKey = key.trim().toLowerCase()
  const keyValid = trimmedKey === '' || KEY_PATTERN.test(trimmedKey)
  const canSubmit = trimmedKey !== '' && name.trim() !== '' && KEY_PATTERN.test(trimmedKey)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (submitting || !canSubmit) return
    setSubmitting(true)
    setError(null)
    try {
      await onCreate(trimmedKey, name.trim())
    } catch (err) {
      setError(errorMessage(err))
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-80 flex items-start justify-center overflow-y-auto bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-item-title"
    >
      <div className="card mt-16 w-full max-w-lg">
        <div className="card-header flex items-center justify-between">
          <h4 id="create-item-title" className="card-title">
            New {noun.toLowerCase()}
          </h4>
          <button type="button" aria-label="Close" onClick={onClose}>
            <Icon icon="x" className="text-xl" />
          </button>
        </div>
        <form onSubmit={submit}>
          <div className="card-body space-y-4">
            {error && (
              <div className="rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
                {error}
              </div>
            )}
            <div>
              <label className="form-label" htmlFor="item-key">
                Key&nbsp;<span className="text-danger">*</span>
              </label>
              <input
                id="item-key"
                type="text"
                className={`form-input font-mono ${!keyValid ? '!border-danger' : ''}`}
                placeholder="e.g. pro-monthly"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                autoFocus
              />
              <p className="text-default-400 mt-1 text-xs">Lowercase alphanumerics and single hyphens. Immutable once created.</p>
              {!keyValid && <p className="mt-1 text-sm text-danger">Key must be lowercase alphanumerics and single hyphens.</p>}
            </div>
            <div>
              <label className="form-label" htmlFor="item-name">
                Name&nbsp;<span className="text-danger">*</span>
              </label>
              <input id="item-name" type="text" className="form-input" placeholder={`e.g. ${noun} name`} value={name} onChange={(e) => setName(e.target.value)} />
            </div>
          </div>
          <div className="card-footer flex items-center justify-end gap-2">
            <button type="button" className="btn bg-light hover:text-primary" onClick={onClose} disabled={submitting}>
              Cancel
            </button>
            <button type="submit" className="btn bg-primary text-white hover:bg-primary-hover" disabled={submitting || !canSubmit}>
              {submitting ? 'Creating…' : `Create ${noun.toLowerCase()}`}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default CreateItemModal
