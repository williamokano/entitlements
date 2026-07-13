// Email-verification confirm page (F-003). Posts the token from the URL query
// (?token=...) to /auth/verify-email and reports success/failure. Adapted from
// the theme's views/auth/basic/success-mail. Dev builds log the token rather
// than emailing it.
import checkMark from '@/assets/images/checkmark.png'
import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { Link, useSearchParams } from 'react-router'
import { ApiError } from '@/lib/api'
import { isAuthenticated, subscribe, verifyEmail } from '@/lib/auth'
import AuthShell from '../components/AuthShell'

type Status = 'verifying' | 'success' | 'error'

const Page = () => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''

  const [status, setStatus] = useState<Status>(token ? 'verifying' : 'error')
  const [message, setMessage] = useState(token ? '' : 'This verification link is missing its token.')
  // Confirm exactly once even under React StrictMode's double-invoke.
  const started = useRef(false)

  useEffect(() => {
    if (!token || started.current) return
    started.current = true
    verifyEmail(token)
      .then(() => setStatus('success'))
      .catch((err) => {
        setStatus('error')
        setMessage(err instanceof ApiError ? err.detail || err.title : 'This verification link is invalid or has expired.')
      })
  }, [token])

  const nextTo = authenticated ? '/' : '/auth/sign-in'
  const nextLabel = authenticated ? 'Back to dashboard' : 'Go to sign in'

  return (
    <AuthShell title="Email verification">
      {status === 'verifying' && (
        <p className="text-default-600 text-center" role="status">
          Verifying your email…
        </p>
      )}

      {status === 'success' && (
        <div className="text-center">
          <div className="mt-3 mb-9">
            <div className="bg-default-50 border-light mx-auto flex size-20 items-center justify-center rounded-full border border-dashed">
              <img src={checkMark} alt="checkmark" className="size-16" />
            </div>
          </div>
          <h4 className="mb-9 text-lg font-bold">Email verified successfully</h4>
          <Link to={nextTo} className="btn bg-primary block w-full py-3 font-semibold text-white hover:bg-primary-hover">
            {nextLabel}
          </Link>
        </div>
      )}

      {status === 'error' && (
        <div className="text-center">
          <p className="text-danger mb-6" role="alert">
            {message}
          </p>
          <Link to={nextTo} className="btn bg-primary block w-full py-3 font-semibold text-white hover:bg-primary-hover">
            {nextLabel}
          </Link>
        </div>
      )}
    </AuthShell>
  )
}

export default Page
