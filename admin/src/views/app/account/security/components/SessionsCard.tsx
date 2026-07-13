import { useCallback, useEffect, useState } from 'react'
import { toast } from 'react-toastify'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { type AuthSession, listSessions, revokeOtherSessions } from '@/lib/auth'

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

const formatDate = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

const SessionsCard = () => {
  const [sessions, setSessions] = useState<AuthSession[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [revoking, setRevoking] = useState(false)

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await listSessions()
      if (signal?.aborted) return
      setSessions(result)
      setError(null)
    } catch (err) {
      if (signal?.aborted) return
      setError(errorMessage(err))
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    // Wrapped in an async IIFE so the effect body performs no synchronous
    // setState — all state mutation happens after the awaited fetch resolves.
    void (async () => {
      await load(controller.signal)
    })()
    return () => controller.abort()
  }, [load])

  const onRevokeOthers = async () => {
    setRevoking(true)
    try {
      await revokeOtherSessions()
      toast.success('Signed out of all other devices.')
      await load()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setRevoking(false)
    }
  }

  const hasOthers = sessions.some((s) => !s.current)

  return (
    <div className="card">
      <div className="card-header flex items-center justify-between">
        <h4 className="card-title flex items-center gap-1.5">
          <Icon icon="devices" className="text-base" />
          Active Sessions
        </h4>
        <button type="button" disabled={!hasOthers || revoking} onClick={onRevokeOthers} className="btn btn-sm bg-danger font-semibold text-white hover:bg-danger-hover">
          {revoking ? 'Signing out…' : 'Log out other devices'}
        </button>
      </div>
      <div className="card-body">
        {loading ? (
          <p className="text-default-500">Loading sessions…</p>
        ) : error ? (
          <p className="text-danger" role="alert">
            {error}
          </p>
        ) : sessions.length === 0 ? (
          <p className="text-default-500">No active sessions.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th className="text-start">Session</th>
                  <th className="text-start">Signed in</th>
                  <th className="text-start">Expires</th>
                </tr>
              </thead>
              <tbody>
                {sessions.map((s) => (
                  <tr key={s.id}>
                    <td>
                      <span className="font-mono text-sm">{s.id.slice(0, 8)}</span>
                      {s.current && <span className="badge bg-success/15 text-success ms-2 rounded px-2 py-0.5 text-xs font-semibold">This device</span>}
                    </td>
                    <td>{formatDate(s.issued_at)}</td>
                    <td>{formatDate(s.expires_at)}</td>
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

export default SessionsCard
