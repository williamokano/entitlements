// Invite modal: email + role. The role list comes from the tenant's roles when
// they are readable and falls back to the seeded system roles otherwise (see
// listInvitableRoles). A duplicate pending invite (409) renders inline.
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { useEffect, useState } from 'react'
import { createInvitation, listInvitableRoles, SYSTEM_ROLES, type Invitation } from '../api'

type Props = {
  tenantId: string
  onClose: () => void
  /** Called with the created invitation so the parent can show it immediately. */
  onInvited: (invitation: Invitation) => void
}

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Could not send the invitation. Please try again.'
}

// Mirrors the backend's rule closely enough to catch typos before the round-trip;
// the server remains the authority (it re-validates and normalizes).
const isValidEmail = (value: string): boolean => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)

const InviteMemberModal = ({ tenantId, onClose, onInvited }: Props) => {
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<string>(SYSTEM_ROLES[2])
  const [roles, setRoles] = useState<string[]>([...SYSTEM_ROLES])
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [emailError, setEmailError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      const available = await listInvitableRoles(controller.signal)
      if (controller.signal.aborted) return
      setRoles(available)
      // Keep the default sensible if the tenant has no "member" role.
      setRole((current) => (available.includes(current) ? current : available[0]))
    })()
    return () => controller.abort()
  }, [])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (submitting) return

    const trimmed = email.trim()
    if (!isValidEmail(trimmed)) {
      setEmailError('Enter a valid email address.')
      return
    }
    setEmailError(null)
    setSubmitting(true)
    setError(null)
    try {
      const invitation = await createInvitation(tenantId, trimmed, role)
      onInvited(invitation)
      onClose()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-80 flex items-start justify-center overflow-y-auto bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="invite-member-title"
    >
      <div className="card mt-16 w-full max-w-lg">
        <div className="card-header flex items-center justify-between">
          <h4 id="invite-member-title" className="card-title">
            Invite member
          </h4>
          <button type="button" aria-label="Close" onClick={onClose}>
            <Icon icon="x" className="text-xl" />
          </button>
        </div>

        <form onSubmit={submit} noValidate>
          <div className="card-body space-y-4">
            {error && (
              <div className="rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
                {error}
              </div>
            )}
            <div>
              <label className="form-label" htmlFor="invite-email">
                Email
              </label>
              <input
                id="invite-email"
                type="email"
                className="form-input"
                placeholder="teammate@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-invalid={emailError !== null}
                aria-describedby={emailError ? 'invite-email-error' : undefined}
                autoFocus
              />
              {emailError && (
                <p id="invite-email-error" className="mt-1 text-xs text-danger" role="alert">
                  {emailError}
                </p>
              )}
            </div>
            <div>
              <label className="form-label" htmlFor="invite-role">
                Role
              </label>
              <select id="invite-role" className="form-select" value={role} onChange={(e) => setRole(e.target.value)}>
                {roles.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
              <p className="text-default-500 mt-1 text-xs">They get this role when they accept the invitation.</p>
            </div>
          </div>
          <div className="card-footer flex items-center justify-end gap-2">
            <button type="button" className="btn bg-light hover:text-primary" onClick={onClose} disabled={submitting}>
              Cancel
            </button>
            <button type="submit" className="btn bg-primary text-white hover:bg-primary-hover" disabled={submitting}>
              {submitting ? 'Sending…' : 'Send invitation'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default InviteMemberModal
