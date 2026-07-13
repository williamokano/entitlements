// Adapted from the theme's views/auth/basic/reset-pass — wired to the real
// password/forgot endpoint (F-003).
import { useSyncExternalStore } from 'react'
import { Link, Navigate } from 'react-router'
import { isAuthenticated, subscribe } from '@/lib/auth'
import AuthShell from '../components/AuthShell'
import Form from './components/Form'

const Page = () => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)

  if (authenticated) {
    return <Navigate to="/" replace />
  }

  return (
    <AuthShell
      title="Forgot Password?"
      subtitle="Enter your email address and we'll send you a link to reset your password."
      footer={
        <p className="text-default-400 mt-7.5 text-center">
          Return to&nbsp;
          <Link to="/auth/sign-in" className="text-primary font-semibold underline underline-offset-4">
            Sign in
          </Link>
        </p>
      }>
      <Form />
    </AuthShell>
  )
}

export default Page
