import { yupResolver } from '@hookform/resolvers/yup'
import { useState } from 'react'
import { type SubmitHandler, useForm } from 'react-hook-form'
import { toast } from 'react-toastify'
import * as yup from 'yup'
import { ApiError } from '@/lib/api'
import { requestPasswordReset } from '@/lib/auth'
import { cn } from '@/utils/helpers'

type FormValues = { email: string }

const schema: yup.ObjectSchema<FormValues> = yup.object({
  email: yup.string().required('Email is required').email('Enter a valid email address'),
})

const Form = () => {
  const [submitted, setSubmitted] = useState(false)
  const [loading, setLoading] = useState(false)

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({ mode: 'onSubmit', resolver: yupResolver(schema) })

  const onSubmit: SubmitHandler<FormValues> = async ({ email }) => {
    setLoading(true)
    try {
      await requestPasswordReset(email)
      // The backend never reveals whether the address exists.
      setSubmitted(true)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.detail || err.title : 'Something went wrong. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  if (submitted) {
    return (
      <p className="text-default-600 text-center" role="status">
        If an account exists for that email, we&apos;ve sent a link to reset your password. Check your inbox.
      </p>
    )
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate>
      <div className="mb-6">
        <label htmlFor="userEmail" className="form-label">
          Email address&nbsp;
          <span className="text-danger">*</span>
        </label>
        <input type="email" id="userEmail" placeholder="you@example.com" className={cn('form-input', errors.email && '!border-danger')} {...register('email')} />
        {errors.email && <p className="text-danger mt-1 text-sm">{errors.email.message}</p>}
      </div>
      <div>
        <button type="submit" disabled={loading} className="btn bg-primary w-full py-3 font-semibold text-white hover:bg-primary-hover">
          {loading ? 'Sending…' : 'Send Request'}
        </button>
      </div>
    </form>
  )
}

export default Form
