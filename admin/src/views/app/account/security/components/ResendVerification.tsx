import { useState } from 'react'
import { toast } from 'react-toastify'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { getSessionEmail, requestEmailVerification } from '@/lib/auth'

/**
 * The authed "request email verification" action. Uses the email captured at
 * sign-in (the backend has no profile endpoint yet); the request always
 * succeeds server-side whether or not the address needs verifying.
 */
const ResendVerification = () => {
  const email = getSessionEmail()
  const [sending, setSending] = useState(false)
  const [sent, setSent] = useState(false)

  const onResend = async () => {
    if (!email) return
    setSending(true)
    try {
      await requestEmailVerification(email)
      setSent(true)
      toast.success('Verification email sent. Check your inbox.')
    } catch (err) {
      toast.error(err instanceof ApiError ? err.detail || err.title : 'Could not send the verification email.')
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="card">
      <div className="card-header">
        <h4 className="card-title flex items-center gap-1.5">
          <Icon icon="mail" className="text-base" />
          Email Verification
        </h4>
      </div>
      <div className="card-body">
        <p className="text-default-600 mb-4">
          {email ? (
            <>
              Send a verification link to <span className="font-semibold">{email}</span> to confirm your email address.
            </>
          ) : (
            'Sign in again to request an email-verification link.'
          )}
        </p>
        <button type="button" disabled={!email || sending} onClick={onResend} className="btn bg-primary font-semibold text-white hover:bg-primary-hover">
          {sending ? 'Sending…' : sent ? 'Resend verification email' : 'Send verification email'}
        </button>
      </div>
    </div>
  )
}

export default ResendVerification
