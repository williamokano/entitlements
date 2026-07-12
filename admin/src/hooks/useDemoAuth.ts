import { useNavigate } from 'react-router'
import { useState } from 'react'
import { useSessionStorage } from 'usehooks-ts'

/**
 * The theme's original dummy auth hook, kept for the demo pages under /demo
 * only. The real app uses hooks/useAuth.ts (backed by lib/auth.ts).
 */
export const useDemoAuth = () => {
  const navigate = useNavigate()

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [token, setToken, removeToken] = useSessionStorage<string | null>('token', null)

  const dummyUser = {
    email: 'admin@example.com',
    password: 'password',
    token: 'auth-token',
  }

  const login = (email: string, password: string) => {
    try {
      setLoading(true)
      setError(null)

      if (email === dummyUser.email && password === dummyUser.password) {
        setToken(dummyUser.token)
        navigate('/demo', { replace: true })
      } else {
        throw new Error('Invalid email or password')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  const logout = () => {
    removeToken()
    navigate('/demo/auth/sign-in', { replace: true })
  }

  const isAuthenticated = token

  return {
    login,
    logout,
    isAuthenticated,
    loading,
    error,
  }
}
