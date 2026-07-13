// Cancel modal: the user chooses between cancelling immediately (a state
// transition to `canceled`) and cancelling at the period end (sets
// cancel_at_period_end, no state change). The choice maps directly to the
// `immediate` boolean the backend's POST /subscription/cancel expects.
import { useState } from 'react'
import Icon from '@/components/wrappers/Icon'
import { errorMessage } from '../../catalog/helpers'
import { formatDate } from '../../catalog/helpers'

type Props = {
  /** Formatted period end, shown as when an at-period-end cancel takes effect. */
  periodEnd: string
  onClose: () => void
  onConfirm: (immediate: boolean, reason: string) => Promise<void>
}

const CancelModal = ({ periodEnd, onClose, onConfirm }: Props) => {
  const [immediate, setImmediate] = useState(false)
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await onConfirm(immediate, reason.trim())
    } catch (err) {
      setError(errorMessage(err))
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-80 flex items-start justify-center overflow-y-auto bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="cancel-title">
      <div className="card mt-16 w-full max-w-lg">
        <div className="card-header flex items-center justify-between">
          <h4 id="cancel-title" className="card-title">
            Cancel subscription
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

            <fieldset className="space-y-3">
              <legend className="form-label mb-1">When should it end?</legend>

              <label className="flex cursor-pointer items-start gap-3 rounded-md border border-default-200 p-3">
                <input type="radio" name="cancel-when" className="form-radio mt-0.5" checked={!immediate} onChange={() => setImmediate(false)} />
                <span>
                  <span className="font-medium">At the end of the current period</span>
                  <span className="text-default-500 block text-sm">Keep access until {formatDate(periodEnd)}; renews no further.</span>
                </span>
              </label>

              <label className="flex cursor-pointer items-start gap-3 rounded-md border border-default-200 p-3">
                <input type="radio" name="cancel-when" className="form-radio mt-0.5" checked={immediate} onChange={() => setImmediate(true)} />
                <span>
                  <span className="font-medium">Immediately</span>
                  <span className="text-default-500 block text-sm">End access now. This cannot be undone.</span>
                </span>
              </label>
            </fieldset>

            <div>
              <label className="form-label" htmlFor="cancel-reason">
                Reason <span className="text-default-400">(optional)</span>
              </label>
              <textarea
                id="cancel-reason"
                className="form-input"
                rows={2}
                placeholder="Why are you cancelling?"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </div>
          </div>
          <div className="card-footer flex items-center justify-end gap-2">
            <button type="button" className="btn bg-light hover:text-primary" onClick={onClose} disabled={submitting}>
              Keep subscription
            </button>
            <button type="submit" className="btn bg-danger text-white hover:bg-danger-hover" disabled={submitting}>
              {submitting ? 'Cancelling…' : immediate ? 'Cancel now' : 'Cancel at period end'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default CancelModal
