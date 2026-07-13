// Adapted from the theme's views/auth/basic/sign-in — wired to the real auth
// store (F-003). Signed-in users are bounced to the app; a post-registration
// redirect shows an "account created, sign in" banner.
import { useSyncExternalStore } from 'react'
import { Link, Navigate, useLocation } from 'react-router'
import { isAuthenticated, subscribe } from '@/lib/auth'
import AuthShell from '../components/AuthShell'
import Form from './components/Form'

type SignInState = { registered?: boolean; email?: string } | null

const Page = () => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)
  const location = useLocation()
  const state = location.state as SignInState

  if (authenticated) {
    return <Navigate to="/" replace />
  }

  return (
    <AuthShell
      title="Welcome"
      subtitle="Let’s get you signed in. Enter your email and password to continue."
      footer={
        <p className="text-default-400 mt-7.5 text-center">
          New here?&nbsp;
          <Link to="/auth/sign-up" className="text-primary font-semibold underline underline-offset-4">
            Create an account
          </Link>
        </p>
      }>
      {state?.registered && (
        <p className="text-success bg-success/10 mb-5 rounded-lg px-4 py-3 text-center text-sm" role="status">
          Account created. Please sign in.
        </p>
      )}
      <Form prefillEmail={state?.email ?? ''} />
    </AuthShell>
  )
}

export default Page
