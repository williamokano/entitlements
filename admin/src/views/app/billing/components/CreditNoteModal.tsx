// Credit-note modal: raises a credit against an invoice. It takes a POSITIVE
// magnitude in integer MINOR units (never a float) and a MANDATORY reason — the
// yup schema blocks submit with an empty reason before any request is made, and
// the backend also enforces both (domain.NewCreditNote). The stored amount is the
// negation of what is entered, so the credit note negates invoice amounts.
import { yupResolver } from '@hookform/resolvers/yup'
import { useState } from 'react'
import { type SubmitHandler, useForm } from 'react-hook-form'
import * as yup from 'yup'
import Icon from '@/components/wrappers/Icon'
import { cn } from '@/utils/helpers'
import { errorMessage } from '../helpers'

type FormValues = {
  amountMinor: number
  reason: string
}

const schema: yup.ObjectSchema<FormValues> = yup.object({
  // Minor units are always whole numbers; keep floats out of the money model.
  amountMinor: yup
    .number()
    .transform((value, original) => (String(original).trim() === '' ? undefined : value))
    .typeError('Enter a whole number of minor units')
    .required('Enter an amount to credit')
    .integer('Amount must be a whole number of minor units')
    .positive('Amount must be greater than zero'),
  reason: yup.string().trim().required('A reason is required'),
})

type Props = {
  /** The invoice currency — used to preview the credited amount. */
  currency: string
  onClose: () => void
  /** Create the credit note with a positive magnitude (minor units) + reason. */
  onSubmit: (amountMinor: number, reason: string) => Promise<void>
}

const CreditNoteModal = ({ currency, onClose, onSubmit }: Props) => {
  const [error, setError] = useState<string | null>(null)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ mode: 'onSubmit', resolver: yupResolver(schema) })

  const submit: SubmitHandler<FormValues> = async ({ amountMinor, reason }) => {
    setError(null)
    try {
      await onSubmit(Math.trunc(amountMinor), reason.trim())
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <div
      className="fixed inset-0 z-80 flex items-start justify-center overflow-y-auto bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="credit-note-title"
    >
      <div className="card mt-16 w-full max-w-lg">
        <div className="card-header flex items-center justify-between">
          <h4 id="credit-note-title" className="card-title">
            Issue credit note
          </h4>
          <button type="button" aria-label="Close" onClick={onClose}>
            <Icon icon="x" className="text-xl" />
          </button>
        </div>
        <form onSubmit={handleSubmit(submit)} noValidate>
          <div className="card-body space-y-4">
            {error && (
              <div className="rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
                {error}
              </div>
            )}

            <div>
              <label className="form-label" htmlFor="cn-amount">
                Amount to credit (minor units)&nbsp;<span className="text-danger">*</span>
              </label>
              <input
                id="cn-amount"
                type="number"
                min={1}
                step={1}
                inputMode="numeric"
                className={cn('form-input', errors.amountMinor && '!border-danger')}
                placeholder="e.g. 1999 for $19.99"
                {...register('amountMinor')}
              />
              {errors.amountMinor ? (
                <p className="text-danger mt-1 text-sm">{errors.amountMinor.message}</p>
              ) : (
                <p className="text-default-400 mt-1 text-xs">Integer minor units (e.g. cents). The credit note lists as a negative amount ({currency}).</p>
              )}
            </div>

            <div>
              <label className="form-label" htmlFor="cn-reason">
                Reason&nbsp;<span className="text-danger">*</span>
              </label>
              <textarea
                id="cn-reason"
                rows={2}
                className={cn('form-input', errors.reason && '!border-danger')}
                placeholder="Why is this credit being issued?"
                {...register('reason')}
              />
              {errors.reason && <p className="text-danger mt-1 text-sm">{errors.reason.message}</p>}
            </div>
          </div>
          <div className="card-footer flex items-center justify-end gap-2">
            <button type="button" className="btn bg-light hover:text-primary" onClick={onClose} disabled={isSubmitting}>
              Cancel
            </button>
            <button type="submit" className="btn bg-primary text-white hover:bg-primary-hover" disabled={isSubmitting}>
              {isSubmitting ? 'Issuing…' : 'Issue credit note'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default CreditNoteModal
