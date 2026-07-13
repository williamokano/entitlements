import { yupResolver } from '@hookform/resolvers/yup'
import { type SubmitHandler, useForm } from 'react-hook-form'
import * as yup from 'yup'
import { cn } from '@/utils/helpers'
import { useAuth } from '@/hooks/useAuth'

type FormValues = {
  email: string
  password: string
  confirmPassword: string
}

// Client-side validation blocks invalid email / short password before any
// request reaches the backend (yup, resolved into react-hook-form).
const schema: yup.ObjectSchema<FormValues> = yup.object({
  email: yup.string().required('Email is required').email('Enter a valid email address'),
  password: yup.string().required('Password is required').min(8, 'Password must be at least 8 characters'),
  confirmPassword: yup
    .string()
    .required('Please confirm your password')
    .oneOf([yup.ref('password')], 'Passwords do not match'),
})

const SignUpForm = () => {
  const { register: registerUser, loading, error } = useAuth()

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    mode: 'onSubmit',
    resolver: yupResolver(schema),
  })

  const onSubmit: SubmitHandler<FormValues> = async ({ email, password }) => {
    await registerUser(email, password)
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate>
      <div className="mb-5">
        <label htmlFor="userEmail" className="form-label">
          Email address&nbsp;
          <span className="text-danger">*</span>
        </label>
        <input type="email" id="userEmail" placeholder="you@example.com" className={cn('form-input', errors.email && '!border-danger')} {...register('email')} />
        {errors.email && <p className="text-danger mt-1 text-sm">{errors.email.message}</p>}
      </div>

      <div className="mb-5">
        <label htmlFor="userPassword" className="form-label">
          Password&nbsp;
          <span className="text-danger">*</span>
        </label>
        <input type="password" id="userPassword" placeholder="••••••••" className={cn('form-input', errors.password && '!border-danger')} {...register('password')} />
        {errors.password ? (
          <p className="text-danger mt-1 text-sm">{errors.password.message}</p>
        ) : (
          <p className="text-default-400 mt-1 text-xs">Use 8+ characters with letters, numbers &amp; symbols.</p>
        )}
      </div>

      <div className="mb-6">
        <label htmlFor="userConfirmPassword" className="form-label">
          Confirm password&nbsp;
          <span className="text-danger">*</span>
        </label>
        <input type="password" id="userConfirmPassword" placeholder="••••••••" className={cn('form-input', errors.confirmPassword && '!border-danger')} {...register('confirmPassword')} />
        {errors.confirmPassword && <p className="text-danger mt-1 text-sm">{errors.confirmPassword.message}</p>}
      </div>

      {error && (
        <p className="text-danger mb-5 text-center" role="alert">
          {error}
        </p>
      )}

      <div>
        <button type="submit" disabled={loading} className="btn bg-primary w-full py-3 font-semibold text-white hover:bg-primary-hover">
          {loading ? 'Creating account…' : 'Create Account'}
        </button>
      </div>
    </form>
  )
}

export default SignUpForm
