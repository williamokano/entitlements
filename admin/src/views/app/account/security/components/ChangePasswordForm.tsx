import { yupResolver } from '@hookform/resolvers/yup'
import { useState } from 'react'
import { type SubmitHandler, useForm } from 'react-hook-form'
import { toast } from 'react-toastify'
import * as yup from 'yup'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { changePassword } from '@/lib/auth'
import { cn } from '@/utils/helpers'

type FormValues = {
  currentPassword: string
  newPassword: string
  confirmPassword: string
}

const schema: yup.ObjectSchema<FormValues> = yup.object({
  currentPassword: yup.string().required('Enter your current password'),
  newPassword: yup.string().required('Enter a new password').min(8, 'Password must be at least 8 characters'),
  confirmPassword: yup
    .string()
    .required('Please confirm your new password')
    .oneOf([yup.ref('newPassword')], 'Passwords do not match'),
})

const ChangePasswordForm = () => {
  const [error, setError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ mode: 'onSubmit', resolver: yupResolver(schema) })

  const onSubmit: SubmitHandler<FormValues> = async ({ currentPassword, newPassword }) => {
    setError(null)
    try {
      await changePassword(currentPassword, newPassword)
      toast.success('Password changed. Other sessions were signed out.')
      reset()
    } catch (err) {
      setError(err instanceof ApiError ? err.detail || err.title : 'Unable to change your password. Please try again.')
    }
  }

  return (
    <div className="card">
      <div className="card-header">
        <h4 className="card-title flex items-center gap-1.5">
          <Icon icon="lock-password" className="text-base" />
          Change Password
        </h4>
      </div>
      <div className="card-body">
        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          <div className="mb-5">
            <label htmlFor="currentPassword" className="form-label">
              Current password&nbsp;
              <span className="text-danger">*</span>
            </label>
            <input type="password" id="currentPassword" placeholder="••••••••" className={cn('form-input', errors.currentPassword && '!border-danger')} {...register('currentPassword')} />
            {errors.currentPassword && <p className="text-danger mt-1 text-sm">{errors.currentPassword.message}</p>}
          </div>

          <div className="mb-5">
            <label htmlFor="newPassword" className="form-label">
              New password&nbsp;
              <span className="text-danger">*</span>
            </label>
            <input type="password" id="newPassword" placeholder="••••••••" className={cn('form-input', errors.newPassword && '!border-danger')} {...register('newPassword')} />
            {errors.newPassword ? (
              <p className="text-danger mt-1 text-sm">{errors.newPassword.message}</p>
            ) : (
              <p className="text-default-400 mt-1 text-xs">Use 8+ characters with letters, numbers &amp; symbols.</p>
            )}
          </div>

          <div className="mb-6">
            <label htmlFor="confirmPassword" className="form-label">
              Confirm new password&nbsp;
              <span className="text-danger">*</span>
            </label>
            <input type="password" id="confirmPassword" placeholder="••••••••" className={cn('form-input', errors.confirmPassword && '!border-danger')} {...register('confirmPassword')} />
            {errors.confirmPassword && <p className="text-danger mt-1 text-sm">{errors.confirmPassword.message}</p>}
          </div>

          {error && (
            <p className="text-danger mb-5" role="alert">
              {error}
            </p>
          )}

          <button type="submit" disabled={isSubmitting} className="btn bg-primary font-semibold text-white hover:bg-primary-hover">
            {isSubmitting ? 'Saving…' : 'Update Password'}
          </button>
        </form>
      </div>
    </div>
  )
}

export default ChangePasswordForm
