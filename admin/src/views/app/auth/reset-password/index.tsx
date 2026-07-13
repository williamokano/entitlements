// Adapted from the theme's views/auth/basic/new-pass — wired to the real
// password/reset endpoint (F-003). The reset token comes from the URL query
// (?token=...); dev builds log the token instead of emailing it.
import { useSyncExternalStore } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router'
import { isAuthenticated, subscribe } from '@/lib/auth'
import AuthShell from '../components/AuthShell'
import Form from './components/Form'

const Page = () => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''

  if (authenticated) {
    return <Navigate to="/" replace />
  }

  return (
    <AuthShell
      title="Set a New Password"
      subtitle="Choose a new password for your account."
      footer={
        <p className="text-default-400 mt-7.5 text-center">
          Return to&nbsp;
          <Link to="/auth/sign-in" className="text-primary font-semibold underline underline-offset-4">
            Sign in
          </Link>
        </p>
      }>
      {token ? (
        <Form token={token} />
      ) : (
        <p className="text-danger text-center" role="alert">
          This reset link is missing its token. Please request a new password reset.
        </p>
      )}
    </AuthShell>
  )
}

export default Page
