// The API-keys card: lists the tenant's keys, opens the create modal (with the
// one-time secret reveal) and the revoke confirmation. All data flows through
// the shared api client (bearer + tenant header + problem+json parsing).
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { useCallback, useEffect, useState } from 'react'
import { listApiKeys, type ApiKey } from '../api'
import CreateApiKeyModal from './CreateApiKeyModal'
import RevokeApiKeyModal from './RevokeApiKeyModal'

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

const formatDate = (value: string | null): string => {
  if (!value) return 'Never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

const ApiKeysCard = () => {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null)

  // Only ever mutates state after the awaited fetch resolves, so the effect
  // body itself performs no synchronous setState.
  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await listApiKeys(signal)
      if (signal?.aborted) return
      setKeys(result)
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

  const retry = () => {
    setLoading(true)
    setError(null)
    void load()
  }

  const handleCreated = () => {
    // Reflect the new key from the server once the reveal is dismissed.
    void load()
  }

  const handleRevoked = (id: string) => {
    // Optimistically drop the revoked row; the secret is already gone server-side.
    setKeys((current) => current.filter((key) => key.id !== id))
    setRevokeTarget(null)
  }

  const isEmpty = !loading && keys.length === 0

  return (
    <div className="card">
      <div className="card-header flex items-center justify-between">
        <div>
          <h4 className="card-title">API keys</h4>
          <p className="text-default-500 mt-1 text-sm">Programmatic access to this organization. Secrets are shown once, at creation.</p>
        </div>
        <button type="button" className="btn bg-primary text-white hover:bg-primary-hover" onClick={() => setCreating(true)}>
          <Icon icon="plus" className="text-base" />
          New API Key
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

        {loading ? (
          <p className="text-default-500 py-8 text-center text-sm">Loading API keys…</p>
        ) : isEmpty ? (
          <div className="py-10 text-center">
            <Icon icon="key" className="text-default-400 mx-auto text-4xl" />
            <h5 className="mt-3 font-medium">No API keys yet</h5>
            <p className="text-default-500 mt-1 text-sm">Create a key to authenticate programmatic requests to the API.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th className="text-start">Name</th>
                  <th className="text-start">Prefix</th>
                  <th className="text-start">Scopes</th>
                  <th className="text-start">Created</th>
                  <th className="text-start">Last used</th>
                  <th className="text-end">Actions</th>
                </tr>
              </thead>
              <tbody>
                {keys.map((key) => (
                  <tr key={key.id}>
                    <td className="font-medium">{key.name}</td>
                    <td>
                      <code className="text-default-600 rounded bg-default-100 px-1.5 py-0.5 text-xs">{key.prefix}</code>
                    </td>
                    <td>
                      {key.scopes.length === 0 ? (
                        <span className="text-default-400">—</span>
                      ) : (
                        <div className="flex flex-wrap gap-1">
                          {key.scopes.map((scope) => (
                            <span key={scope} className="badge badge-label bg-info/15 text-info">
                              {scope}
                            </span>
                          ))}
                        </div>
                      )}
                    </td>
                    <td className="text-default-500 text-sm">{formatDate(key.created_at)}</td>
                    <td className="text-default-500 text-sm">{formatDate(key.last_used_at)}</td>
                    <td className="text-end">
                      <button
                        type="button"
                        className="btn btn-icon btn-sm border border-default-300 hover:border-danger hover:text-danger"
                        aria-label={`Revoke ${key.name}`}
                        onClick={() => setRevokeTarget(key)}
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

      {creating && <CreateApiKeyModal onClose={() => setCreating(false)} onCreated={handleCreated} />}
      {revokeTarget && <RevokeApiKeyModal apiKey={revokeTarget} onClose={() => setRevokeTarget(null)} onRevoked={handleRevoked} />}
    </div>
  )
}

export default ApiKeysCard
