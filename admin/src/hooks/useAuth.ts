import { useState, useSyncExternalStore } from 'react'
import { useNavigate } from 'react-router'
import { ApiError } from '@/lib/api'
import {
  isAuthenticated as isAuthenticatedSnapshot,
  login as authLogin,
  logout as authLogout,
  register as authRegister,
  subscribe,
} from '@/lib/auth'

const LOGIN_RATE_LIMITED = 'Too many attempts. Please wait a moment and try again.'

/** Turns any thrown value into a user-facing message; 429 gets a friendlier line. */
const messageForError = (err: unknown, fallback: string): string => {
  if (err instanceof ApiError) {
    if (err.status === 429) return LOGIN_RATE_LIMITED
    return err.detail || err.title || fallback
  }
  return fallback
}

/**
 * Real auth hook for the product screens — wraps the auth store in lib/auth.ts.
 * (The vendored theme demo keeps its own dummy hook: hooks/useDemoAuth.ts.)
 */
export const useAuth = () => {
  const navigate = useNavigate()

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const isAuthenticated = useSyncExternalStore(subscribe, isAuthenticatedSnapshot)

  /**
   * Sign in and land on `redirectTo` — the route the user was headed for before
   * a guard bounced them (RequireAuth stashes it in `state.from`), or the
   * invitation they were asked to accept. Defaults to the app root.
   */
  const login = async (email: string, password: string, redirectTo = '/') => {
    setLoading(true)
    setError(null)
    try {
      await authLogin(email, password)
      navigate(redirectTo, { replace: true })
    } catch (err) {
      setError(messageForError(err, 'Unable to sign in. Please try again.'))
    } finally {
      setLoading(false)
    }
  }

  /**
   * Create an account and, on success, drop the user on the sign-in page with a
   * banner nudging them to sign in (the backend issues no tokens on register).
   * Returns true on success so callers can react.
   */
  const register = async (email: string, password: string): Promise<boolean> => {
    setLoading(true)
    setError(null)
    try {
      await authRegister(email, password)
      navigate('/auth/sign-in', { replace: true, state: { registered: true, email } })
      return true
    } catch (err) {
      setError(messageForError(err, 'Unable to create your account. Please try again.'))
      return false
    } finally {
      setLoading(false)
    }
  }

  const logout = async () => {
    await authLogout()
    navigate('/auth/sign-in', { replace: true })
  }

  return {
    login,
    register,
    logout,
    isAuthenticated,
    loading,
    error,
  }
}
