// The members card: lists the tenant's active members and removes one after an
// explicit confirmation. A member's email is the address they were invited by;
// memberships written before the backend stored it fall back to the user id.
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { confirmAction } from '@/lib/confirm'
import { useCallback, useEffect, useState } from 'react'
import { listMembers, removeMember, type Member } from '../api'

type Props = {
  tenantId: string
}

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

/** What to call a member: their invited email, or the user id when we have none. */
const memberLabel = (member: Member): string => member.email || member.user_id

const roleClass = (role: string): string => {
  if (role === 'owner') return 'bg-primary/15 text-primary'
  if (role === 'admin') return 'bg-info/15 text-info'
  return 'bg-default-200 text-default-700'
}

const MembersCard = ({ tenantId }: Props) => {
  const [members, setMembers] = useState<Member[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [removing, setRemoving] = useState<string | null>(null)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const result = await listMembers(tenantId, signal)
        if (signal?.aborted) return
        setMembers(result)
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
    // Async IIFE so the effect body performs no synchronous setState — every
    // mutation happens after the awaited fetch resolves.
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

  // Destructive: confirm before the request, never after.
  const remove = async (member: Member) => {
    const confirmed = await confirmAction({
      title: 'Remove member?',
      text: `${memberLabel(member)} will immediately lose access to this organization.`,
      confirmText: 'Remove',
    })
    if (!confirmed) return

    setRemoving(member.user_id)
    setError(null)
    try {
      await removeMember(tenantId, member.user_id)
      setMembers((current) => current.filter((m) => m.user_id !== member.user_id))
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setRemoving(null)
    }
  }

  const isEmpty = !loading && members.length === 0

  return (
    <div className="card">
      <div className="card-header">
        <h4 className="card-title">Members</h4>
        <p className="text-default-500 mt-1 text-sm">People with access to this organization.</p>
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

        {loading ? (
          <p className="text-default-500 py-8 text-center text-sm">Loading members…</p>
        ) : isEmpty ? (
          <div className="py-10 text-center">
            <Icon icon="users" className="text-default-400 mx-auto text-4xl" />
            <h5 className="mt-3 font-medium">No members yet</h5>
            <p className="text-default-500 mt-1 text-sm">Invite someone below — they become a member once they accept.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th className="text-start">Member</th>
                  <th className="text-start">Role</th>
                  <th className="text-start">Status</th>
                  <th className="text-end">Actions</th>
                </tr>
              </thead>
              <tbody>
                {members.map((member) => (
                  <tr key={member.user_id}>
                    <td>
                      <span className="font-medium">{memberLabel(member)}</span>
                      {member.email && <p className="text-default-400 mt-0.5 font-mono text-xs">{member.user_id}</p>}
                    </td>
                    <td>
                      <span className={`badge badge-label ${roleClass(member.role)}`}>{member.role}</span>
                    </td>
                    <td>
                      <span className="badge badge-label bg-success/15 text-success">{member.status}</span>
                    </td>
                    <td className="text-end">
                      <button
                        type="button"
                        className="btn btn-icon btn-sm border border-default-300 hover:border-danger hover:text-danger"
                        aria-label={`Remove ${memberLabel(member)}`}
                        disabled={removing === member.user_id}
                        onClick={() => void remove(member)}
                      >
                        <Icon icon="trash" className="text-base" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

export default MembersCard
