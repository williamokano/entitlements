/**
 * API client — a single fetch wrapper for the Go backend.
 *
 * - Prefixes every path with the runtime `apiBaseUrl`.
 * - JSON-encodes bodies and decodes JSON responses.
 * - Attaches `Authorization: Bearer <access>` when a session exists.
 * - Attaches the tenant header (X-Tenant-ID) in header tenant mode.
 * - Parses RFC 7807 application/problem+json errors into a typed ApiError.
 * - On a 401 it transparently attempts EXACTLY ONE token refresh
 *   (`POST /api/v1/auth/refresh`) and retries the original request once;
 *   if the refresh is rejected (rotation/reuse) it hard-logs-out.
 * - `withIdempotencyKey()` adds an `Idempotency-Key` (uuid) for mutating calls.
 */

import { appConfig } from './config'
import { clearTokens, getAccessToken, getRefreshToken, setTokens } from './tokens'

export class ApiError extends Error {
  readonly status: number
  readonly title: string
  readonly detail: string

  constructor(status: number, title: string, detail: string) {
    super(detail || title || `HTTP ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.title = title
    this.detail = detail
  }
}

export type ApiRequestOptions = {
  method?: string
  body?: unknown
  headers?: Record<string, string>
  /** Attach the bearer token (and 401-refresh-retry). Default true. */
  auth?: boolean
  signal?: AbortSignal
}

/** Returns headers carrying a fresh Idempotency-Key for a mutating call. */
export function withIdempotencyKey(headers: Record<string, string> = {}): Record<string, string> {
  return { ...headers, 'Idempotency-Key': crypto.randomUUID() }
}

const isProblemJson = (res: Response) => (res.headers.get('content-type') ?? '').includes('application/problem+json')

async function toApiError(res: Response): Promise<ApiError> {
  let title = res.statusText || 'Request failed'
  let detail = ''
  let status = res.status
  if (isProblemJson(res)) {
    try {
      const problem = (await res.json()) as { status?: number; title?: string; detail?: string }
      status = problem.status ?? status
      title = problem.title ?? title
      detail = problem.detail ?? ''
    } catch {
      // fall through with the HTTP status line
    }
  }
  return new ApiError(status, title, detail)
}

// Single-flight refresh: concurrent 401s share one refresh request.
let refreshInFlight: Promise<boolean> | null = null

async function refreshTokens(): Promise<boolean> {
  const refreshToken = getRefreshToken()
  if (!refreshToken) return false
  try {
    const res = await fetch(`${appConfig().apiBaseUrl}/api/v1/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
    if (!res.ok) return false
    const pair = (await res.json()) as { access_token: string; refresh_token: string }
    setTokens({ accessToken: pair.access_token, refreshToken: pair.refresh_token })
    return true
  } catch {
    return false
  }
}

function refreshOnce(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = refreshTokens().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

function buildHeaders(options: ApiRequestOptions): Headers {
  const config = appConfig()
  const headers = new Headers(options.headers)
  if (options.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (config.tenantMode === 'header' && config.tenantSlug && !headers.has('X-Tenant-ID')) {
    headers.set('X-Tenant-ID', config.tenantSlug)
  }
  if (options.auth !== false) {
    const access = getAccessToken()
    if (access) headers.set('Authorization', `Bearer ${access}`)
  }
  return headers
}

async function doFetch(path: string, options: ApiRequestOptions): Promise<Response> {
  return fetch(`${appConfig().apiBaseUrl}${path}`, {
    method: options.method ?? 'GET',
    headers: buildHeaders(options),
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
    signal: options.signal,
  })
}

/**
 * Perform an API request. Resolves with the decoded JSON body (or undefined
 * for 204/empty responses); rejects with ApiError on any non-2xx response.
 */
export async function apiFetch<T = unknown>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  let res = await doFetch(path, options)

  // Transparent refresh-and-retry: exactly once, only for authenticated
  // requests, and never for the auth endpoints themselves.
  if (res.status === 401 && options.auth !== false && getAccessToken() && !path.startsWith('/api/v1/auth/')) {
    const refreshed = await refreshOnce()
    if (!refreshed) {
      // Refresh rejected (rotation/reuse or expired) — hard logout.
      clearTokens()
      throw await toApiError(res)
    }
    res = await doFetch(path, options)
  }

  if (!res.ok) {
    throw await toApiError(res)
  }

  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}
