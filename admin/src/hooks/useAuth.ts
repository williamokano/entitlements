import { useState, useSyncExternalStore } from 'react'
import { useNavigate } from 'react-router'
import { ApiError } from '@/lib/api'
import { isAuthenticated as isAuthenticatedSnapshot, login as authLogin, logout as authLogout, subscribe } from '@/lib/auth'

/**
 * Real auth hook for the product screens — wraps the auth store in lib/auth.ts.
 * (The vendored theme demo keeps its own dummy hook: hooks/useDemoAuth.ts.)
 */
export const useAuth = () => {
  const navigate = useNavigate()

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const isAuthenticated = useSyncExternalStore(subscribe, isAuthenticatedSnapshot)

  const login = async (email: string, password: string) => {
    setLoading(true)
    setError(null)
    try {
      await authLogin(email, password)
      navigate('/', { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.detail || err.title)
      } else {
        setError('Unable to sign in. Please try again.')
      }
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
    logout,
    isAuthenticated,
    loading,
    error,
  }
}
