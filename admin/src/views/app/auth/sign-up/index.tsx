// Adapted from the theme's views/auth/basic/sign-up — wired to the real auth
// store (F-003). On success the user is redirected to sign-in.
import { useSyncExternalStore } from 'react'
import { Link, Navigate } from 'react-router'
import { isAuthenticated, subscribe } from '@/lib/auth'
import AuthShell from '../components/AuthShell'
import SignUpForm from './components/SignUpForm'

const Page = () => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)

  if (authenticated) {
    return <Navigate to="/" replace />
  }

  return (
    <AuthShell
      title="Create New Account"
      subtitle="Let’s get you started. Create your account by entering your details below."
      footer={
        <p className="text-default-400 mt-7.5 text-center">
          Already have an account?&nbsp;
          <Link to="/auth/sign-in" className="text-primary font-semibold underline underline-offset-4">
            Sign in
          </Link>
        </p>
      }>
      <SignUpForm />
    </AuthShell>
  )
}

export default Page
