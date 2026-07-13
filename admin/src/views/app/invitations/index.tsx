// Invitation accept/decline (F-005) — the page an invitee lands on from their
// email. It is public: an invitee typically arrives signed out, and may not even
// have an account yet.
//
// There is no invitation token. The backend authorizes accept/decline by
// matching the *signed-in user's* email to the invitation, keyed by tenant +
// invitation id — which is why the link carries both ids and why the page can
// show nothing about the invitation until the user is authenticated (there is no
// endpoint to read one anonymously, by design: it would leak the invited email).
import { useState, useSyncExternalStore } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router'
import { ApiError } from '@/lib/api'
import { getSessionEmail, isAuthenticated, subscribe } from '@/lib/auth'
import { rememberAndSelect } from '@/lib/tenant'
import AuthShell from '../auth/components/AuthShell'
import { acceptInvitation, declineInvitation } from '../members/api'
import { getTenant } from '../tenant/api'

type Outcome = { kind: 'declined' } | { kind: 'error'; title: string; detail: string }

// The backend's failure modes, turned into something an invitee can act on.
const describeError = (err: unknown, email: string | null): { title: string; detail: string } => {
  if (err instanceof ApiError) {
    if (err.status === 403) {
      return {
        title: 'This invitation is for someone else',
        detail: email
          ? `It was sent to a different email address than the one you are signed in with (${email}). Sign in with the invited address to accept it.`
          : 'It was sent to a different email address than the one you are signed in with. Sign in with the invited address to accept it.',
      }
    }
    if (err.status === 409) {
      return {
        title: 'This invitation is no longer valid',
        detail: 'It has expired, or it was already accepted or declined. Ask an administrator to send you a new one.',
      }
    }
    if (err.status === 404) {
      return { title: 'Invitation not found', detail: 'The link may be incorrect or the invitation may have been removed.' }
    }
    return { title: 'Something went wrong', detail: err.detail || err.title || `Request failed (${err.status})` }
  }
  return { title: 'Something went wrong', detail: 'Please check your connection and try again.' }
}

const Page = () => {
  const { tenantId, invId } = useParams<{ tenantId: string; invId: string }>()
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)
  const location = useLocation()
  const navigate = useNavigate()

  const [busy, setBusy] = useState(false)
  const [outcome, setOutcome] = useState<Outcome | null>(null)

  const email = getSessionEmail()

  if (!tenantId || !invId) {
    return (
      <AuthShell title="Invalid invitation link" subtitle="This link is missing information. Please use the link from your invitation email.">
        <Link to="/" className="btn bg-primary text-white hover:bg-primary-hover w-full">
          Go to the app
        </Link>
      </AuthShell>
    )
  }

  const accept = async () => {
    setBusy(true)
    setOutcome(null)
    try {
      await acceptInvitation(tenantId, invId)
      // The invitee is now a member — make this their active tenant so they land
      // in the organization they were invited to. Membership is what grants the
      // read, so the tenant is only fetchable now, after the accept.
      try {
        const tenant = await getTenant(tenantId)
        rememberAndSelect({ id: tenant.id, name: tenant.name, slug: tenant.slug })
      } catch {
        // The membership is real either way; don't strand the user on a failed
        // lookup. The tenant switcher recovers the details on the next load.
        rememberAndSelect({ id: tenantId, name: tenantId, slug: tenantId })
      }
      void navigate('/', { replace: true })
    } catch (err) {
      setOutcome({ kind: 'error', ...describeError(err, email) })
    } finally {
      setBusy(false)
    }
  }

  const decline = async () => {
    setBusy(true)
    setOutcome(null)
    try {
      await declineInvitation(tenantId, invId)
      setOutcome({ kind: 'declined' })
    } catch (err) {
      setOutcome({ kind: 'error', ...describeError(err, email) })
    } finally {
      setBusy(false)
    }
  }

  if (outcome?.kind === 'declined') {
    return (
      <AuthShell title="Invitation declined" subtitle="You have declined this invitation. Nothing else to do here.">
        <Link to="/auth/sign-in" className="btn bg-primary text-white hover:bg-primary-hover w-full">
          Go to sign in
        </Link>
      </AuthShell>
    )
  }

  // Anonymous invitee: accepting requires an identity to attach the membership
  // to, so send them to sign in and bring them straight back here afterwards.
  if (!authenticated) {
    return (
      <AuthShell
        title="You have been invited"
        subtitle="Sign in to accept this invitation. If you do not have an account yet, create one with the email address the invitation was sent to."
        footer={
          <p className="text-default-400 mt-7.5 text-center">
            No account yet?&nbsp;
            <Link to="/auth/sign-up" className="text-primary font-semibold underline underline-offset-4">
              Create an account
            </Link>
          </p>
        }
      >
        <button
          type="button"
          className="btn bg-primary text-white hover:bg-primary-hover w-full"
          onClick={() => void navigate('/auth/sign-in', { state: { from: location } })}
        >
          Sign in to accept
        </button>
      </AuthShell>
    )
  }

  return (
    <AuthShell
      title="You have been invited"
      subtitle={email ? `Accept to join this organization as ${email}.` : 'Accept to join this organization.'}
    >
      {outcome?.kind === 'error' && (
        <div className="mb-5 rounded-lg bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          <p className="font-semibold">{outcome.title}</p>
          <p className="mt-1">{outcome.detail}</p>
        </div>
      )}
      <div className="flex flex-col gap-2">
        <button type="button" className="btn bg-primary text-white hover:bg-primary-hover w-full" disabled={busy} onClick={() => void accept()}>
          {busy ? 'Working…' : 'Accept invitation'}
        </button>
        <button type="button" className="btn bg-light hover:text-primary w-full" disabled={busy} onClick={() => void decline()}>
          Decline
        </button>
      </div>
    </AuthShell>
  )
}

export default Page
