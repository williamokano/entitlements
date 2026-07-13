import { type ChangeEvent, type FormEvent, useState } from 'react'
import { Link } from 'react-router'
import { useAuth } from '@/hooks/useAuth'

type FormProps = {
  /** Prefilled email, e.g. carried over from the sign-up redirect. */
  prefillEmail?: string
}

const Form = ({ prefillEmail = '' }: FormProps) => {
  const { login, loading, error } = useAuth()

  const [form, setForm] = useState({
    email: prefillEmail,
    password: '',
  })

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target
    setForm((prev) => ({ ...prev, [name]: value }))
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    await login(form.email, form.password)
  }

  return (
    <form onSubmit={handleSubmit}>
      <div className="mb-5">
        <label htmlFor="userEmail" className="form-label">
          Email address&nbsp;
          <span className="text-danger">*</span>
        </label>
        <div className="input-group">
          <input
            type="email"
            name="email"
            className="form-input"
            id="userEmail"
            placeholder="you@example.com"
            value={form.email}
            required
            onChange={handleChange}
          />
        </div>
      </div>

      <div className="mb-5">
        <div className="flex items-center justify-between">
          <label htmlFor="userPassword" className="form-label">
            Password&nbsp;
            <span className="text-danger">*</span>
          </label>
          <Link to="/auth/forgot-password" className="text-primary text-sm font-medium underline underline-offset-4">
            Forgot password?
          </Link>
        </div>
        <div className="input-group">
          <input
            type="password"
            name="password"
            className="form-input"
            id="userPassword"
            placeholder="••••••••"
            value={form.password}
            required
            onChange={handleChange}
          />
        </div>
      </div>

      {error && (
        <p className="text-danger mb-5 text-center" role="alert">
          {error}
        </p>
      )}
      <div>
        <button type="submit" disabled={loading} className="btn bg-primary w-full py-3 font-semibold text-white hover:bg-primary-hover">
          {loading ? 'Signing in…' : 'Sign In'}
        </button>
      </div>
    </form>
  )
}

export default Form
