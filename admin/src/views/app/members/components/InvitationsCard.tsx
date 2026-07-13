// Pending invitations: the list the backend still considers open, plus the
// invite action and a per-row resend (which extends the invitation's expiry).
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { useCallback, useEffect, useState } from 'react'
import { listInvitations, resendInvitation, type Invitation } from '../api'
import InviteMemberModal from './InviteMemberModal'

type Props = {
  tenantId: string
}

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

const formatDate = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

const InvitationsCard = ({ tenantId }: Props) => {
  const [invitations, setInvitations] = useState<Invitation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [inviting, setInviting] = useState(false)
  const [resending, setResending] = useState<string | null>(null)
  const [resent, setResent] = useState<string | null>(null)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const result = await listInvitations(tenantId, signal)
        if (signal?.aborted) return
        setInvitations(result)
        setError(null)
      } catch (err) {
        if (signal?.aborted) return
        setError(errorMessage(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [tenantId],
  )

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  const retry = () => {
    setLoading(true)
    setError(null)
    void load()
  }

  const handleInvited = (invitation: Invitation) => {
    setInvitations((current) => [invitation, ...current])
  }

  const resend = async (invitation: Invitation) => {
    setResending(invitation.id)
    setResent(null)
    setError(null)
    try {
      const updated = await resendInvitation(tenantId, invitation.id)
      // The resend extends the expiry — reflect the server's new value.
      setInvitations((current) => current.map((i) => (i.id === updated.id ? updated : i)))
      setResent(invitation.email)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setResending(null)
    }
  }

  const isEmpty = !loading && invitations.length === 0

  return (
    <div className="card mt-6">
      <div className="card-header flex items-center justify-between">
        <div>
          <h4 className="card-title">Pending invitations</h4>
          <p className="text-default-500 mt-1 text-sm">People who have been invited but have not accepted yet.</p>
        </div>
        <button type="button" className="btn bg-primary text-white hover:bg-primary-hover" onClick={() => setInviting(true)}>
          <Icon icon="plus" className="text-base" />
          Invite member
        </button>
      </div>

      <div className="card-body">
        {error && (
          <div className="mb-4 flex items-center justify-between rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
            <span>{error}</span>
            <button type="button" className="btn btn-sm bg-danger text-white hover:bg-danger-hover" onClick={retry}>
              Retry
            </button>
          </div>
        )}
        {resent && (
          <div className="mb-4 rounded-md bg-success/10 px-4 py-3 text-sm text-success" role="status">
            Invitation resent to {resent}.
          </div>
        )}

        {loading ? (
          <p className="text-default-500 py-8 text-center text-sm">Loading invitations…</p>
        ) : isEmpty ? (
          <div className="py-10 text-center">
            <Icon icon="mail" className="text-default-400 mx-auto text-4xl" />
            <h5 className="mt-3 font-medium">No pending invitations</h5>
            <p className="text-default-500 mt-1 text-sm">Invite someone to join this organization.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th className="text-start">Email</th>
                  <th className="text-start">Role</th>
                  <th className="text-start">Invited</th>
                  <th className="text-start">Expires</th>
                  <th className="text-end">Actions</th>
                </tr>
              </thead>
              <tbody>
                {invitations.map((invitation) => (
                  <tr key={invitation.id}>
                    <td className="font-medium">{invitation.email}</td>
                    <td>
                      <span className="badge badge-label bg-default-200 text-default-700">{invitation.role}</span>
                    </td>
                    <td className="text-default-500 text-sm">{formatDate(invitation.created_at)}</td>
                    <td className="text-default-500 text-sm">{formatDate(invitation.expires_at)}</td>
                    <td className="text-end">
                      <button
                        type="button"
                        className="btn btn-sm border border-default-300 hover:border-primary hover:text-primary"
                        aria-label={`Resend invitation to ${invitation.email}`}
                        disabled={resending === invitation.id}
                        onClick={() => void resend(invitation)}
                      >
                        {resending === invitation.id ? 'Resending…' : 'Resend'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {inviting && <InviteMemberModal tenantId={tenantId} onClose={() => setInviting(false)} onInvited={handleInvited} />}
    </div>
  )
}

export default InvitationsCard
