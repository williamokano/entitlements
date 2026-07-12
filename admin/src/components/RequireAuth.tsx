import { type ReactNode, useSyncExternalStore } from 'react'
import { Navigate, useLocation } from 'react-router'
import { isAuthenticated, subscribe } from '@/lib/auth'

/**
 * Route guard: renders children only for authenticated users; anonymous
 * visitors are redirected to /auth/sign-in (remembering where they came from).
 */
const RequireAuth = ({ children }: { children: ReactNode }) => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)
  const location = useLocation()

  if (!authenticated) {
    return <Navigate to="/auth/sign-in" replace state={{ from: location }} />
  }

  return <>{children}</>
}

export default RequireAuth
