import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { ApiError, apiFetch, withIdempotencyKey } from './api'
import { clearTokens, getTokens, setTokens } from './tokens'

const BASE = 'http://api.test'

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  clearTokens()
  window.__APP_CONFIG__ = {
    apiBaseUrl: BASE,
    tenantMode: 'header',
    tenantSlug: 'tenant-123',
    appName: 'Test',
    enableDemo: false,
  }
})

describe('apiFetch headers', () => {
  it('attaches the bearer token and the tenant header in header mode', async () => {
    setTokens({ accessToken: 'access-1', refreshToken: 'refresh-1' })
    let captured: Headers | undefined
    server.use(
      http.get(`${BASE}/api/v1/things`, ({ request }) => {
        captured = request.headers
        return HttpResponse.json({ ok: true })
      }),
    )

    await apiFetch(`/api/v1/things`)

    expect(captured?.get('authorization')).toBe('Bearer access-1')
    expect(captured?.get('x-tenant-id')).toBe('tenant-123')
  })

  it('omits the tenant header in subdomain mode', async () => {
    window.__APP_CONFIG__ = { ...window.__APP_CONFIG__, tenantMode: 'subdomain' }
    let captured: Headers | undefined
    server.use(
      http.get(`${BASE}/api/v1/things`, ({ request }) => {
        captured = request.headers
        return HttpResponse.json({ ok: true })
      }),
    )

    await apiFetch(`/api/v1/things`)

    expect(captured?.get('x-tenant-id')).toBeNull()
  })

  it('withIdempotencyKey adds a uuid Idempotency-Key header', () => {
    const headers = withIdempotencyKey()
    expect(headers['Idempotency-Key']).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i)
    // and each call generates a fresh key
    expect(withIdempotencyKey()['Idempotency-Key']).not.toBe(headers['Idempotency-Key'])
  })
})

describe('apiFetch problem+json', () => {
  it('parses application/problem+json into a typed ApiError', async () => {
    server.use(
      http.get(`${BASE}/api/v1/things`, () =>
        HttpResponse.json(
          { status: 422, title: 'Validation failed', detail: 'name is required' },
          { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    const err = await apiFetch(`/api/v1/things`).catch((e) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(422)
    expect(err.title).toBe('Validation failed')
    expect(err.detail).toBe('name is required')
  })
})

describe('apiFetch 401 refresh-and-retry', () => {
  it('a 401 triggers exactly one refresh and retries the original request', async () => {
    setTokens({ accessToken: 'stale', refreshToken: 'refresh-1' })
    let refreshCalls = 0
    let apiCalls = 0

    server.use(
      http.post(`${BASE}/api/v1/auth/refresh`, async ({ request }) => {
        refreshCalls++
        const body = (await request.json()) as { refresh_token: string }
        expect(body.refresh_token).toBe('refresh-1')
        return HttpResponse.json({
          access_token: 'fresh',
          refresh_token: 'refresh-2',
          token_type: 'Bearer',
        })
      }),
      http.get(`${BASE}/api/v1/things`, ({ request }) => {
        apiCalls++
        if (request.headers.get('authorization') !== 'Bearer fresh') {
          return HttpResponse.json(
            { status: 401, title: 'Unauthorized', detail: 'token expired' },
            { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
          )
        }
        return HttpResponse.json({ ok: true })
      }),
    )

    const result = await apiFetch<{ ok: boolean }>(`/api/v1/things`)

    expect(result.ok).toBe(true)
    expect(refreshCalls).toBe(1)
    expect(apiCalls).toBe(2) // original + exactly one retry
    expect(getTokens()).toEqual({ accessToken: 'fresh', refreshToken: 'refresh-2' })
  })

  it('does not retry more than once when the API keeps returning 401', async () => {
    setTokens({ accessToken: 'stale', refreshToken: 'refresh-1' })
    let refreshCalls = 0
    let apiCalls = 0

    server.use(
      http.post(`${BASE}/api/v1/auth/refresh`, () => {
        refreshCalls++
        return HttpResponse.json({ access_token: 'fresh', refresh_token: 'refresh-2', token_type: 'Bearer' })
      }),
      http.get(`${BASE}/api/v1/things`, () => {
        apiCalls++
        return HttpResponse.json(
          { status: 401, title: 'Unauthorized', detail: 'nope' },
          { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
        )
      }),
    )

    const err = await apiFetch(`/api/v1/things`).catch((e) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(401)
    expect(refreshCalls).toBe(1)
    expect(apiCalls).toBe(2)
  })

  it('a failed refresh clears the session (hard logout)', async () => {
    setTokens({ accessToken: 'stale', refreshToken: 'reused-refresh' })

    server.use(
      http.post(`${BASE}/api/v1/auth/refresh`, () =>
        HttpResponse.json(
          { status: 401, title: 'Unauthorized', detail: 'refresh token reuse detected' },
          { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
      http.get(`${BASE}/api/v1/things`, () =>
        HttpResponse.json(
          { status: 401, title: 'Unauthorized', detail: 'token expired' },
          { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    const err = await apiFetch(`/api/v1/things`).catch((e) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(401)
    expect(getTokens()).toBeNull()
  })

  it('does not attempt a refresh for anonymous requests', async () => {
    let refreshCalls = 0
    server.use(
      http.post(`${BASE}/api/v1/auth/refresh`, () => {
        refreshCalls++
        return HttpResponse.json({ access_token: 'x', refresh_token: 'y', token_type: 'Bearer' })
      }),
      http.post(`${BASE}/api/v1/auth/login`, () =>
        HttpResponse.json(
          { status: 401, title: 'Unauthorized', detail: 'invalid credentials' },
          { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    const err = await apiFetch(`/api/v1/auth/login`, { method: 'POST', body: { email: 'a@b.c', password: 'x' }, auth: false }).catch((e) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(refreshCalls).toBe(0)
  })
})
