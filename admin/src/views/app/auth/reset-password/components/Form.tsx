import { yupResolver } from '@hookform/resolvers/yup'
import { useState } from 'react'
import { type SubmitHandler, useForm } from 'react-hook-form'
import { useNavigate } from 'react-router'
import { toast } from 'react-toastify'
import * as yup from 'yup'
import { ApiError } from '@/lib/api'
import { resetPassword } from '@/lib/auth'
import { cn } from '@/utils/helpers'

type FormProps = { token: string }

type FormValues = {
  password: string
  confirmPassword: string
}

const schema: yup.ObjectSchema<FormValues> = yup.object({
  password: yup.string().required('Password is required').min(8, 'Password must be at least 8 characters'),
  confirmPassword: yup
    .string()
    .required('Please confirm your password')
    .oneOf([yup.ref('password')], 'Passwords do not match'),
})

const Form = ({ token }: FormProps) => {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({ mode: 'onSubmit', resolver: yupResolver(schema) })

  const onSubmit: SubmitHandler<FormValues> = async ({ password }) => {
    setLoading(true)
    setError(null)
    try {
      await resetPassword(token, password)
      toast.success('Your password has been reset. Please sign in.')
      navigate('/auth/sign-in', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.detail || err.title : 'Unable to reset your password. The link may have expired.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate>
      <div className="mb-5">
        <label htmlFor="newPassword" className="form-label">
          New password&nbsp;
          <span className="text-danger">*</span>
        </label>
        <input type="password" id="newPassword" placeholder="••••••••" className={cn('form-input', errors.password && '!border-danger')} {...register('password')} />
        {errors.password ? (
          <p className="text-danger mt-1 text-sm">{errors.password.message}</p>
        ) : (
          <p className="text-default-400 mt-1 text-xs">Use 8+ characters with letters, numbers &amp; symbols.</p>
        )}
      </div>

      <div className="mb-6">
        <label htmlFor="confirmNewPassword" className="form-label">
          Confirm new password&nbsp;
          <span className="text-danger">*</span>
        </label>
        <input type="password" id="confirmNewPassword" placeholder="••••••••" className={cn('form-input', errors.confirmPassword && '!border-danger')} {...register('confirmPassword')} />
        {errors.confirmPassword && <p className="text-danger mt-1 text-sm">{errors.confirmPassword.message}</p>}
      </div>

      {error && (
        <p className="text-danger mb-5 text-center" role="alert">
          {error}
        </p>
      )}

      <div>
        <button type="submit" disabled={loading} className="btn bg-primary w-full py-3 font-semibold text-white hover:bg-primary-hover">
          {loading ? 'Updating…' : 'Update Password'}
        </button>
      </div>
    </form>
  )
}

export default Form
