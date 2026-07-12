import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import ApiKeysCard from './components/ApiKeysCard'

const BASE = 'http://api.test'
const KEYS_PATH = `${BASE}/api/v1/api-keys`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = {
    apiBaseUrl: BASE,
    tenantMode: 'header',
    tenantSlug: 'tenant-123',
    appName: 'Test',
    enableDemo: false,
  }
  setTokens({ accessToken: 'access-1', refreshToken: 'refresh-1' })
})

const listResponse = (keys: unknown[]) => HttpResponse.json({ api_keys: keys })

describe('ApiKeysCard — list', () => {
  it('renders keys returned by GET', async () => {
    server.use(
      http.get(KEYS_PATH, () =>
        listResponse([
          { id: 'k1', prefix: 'ak_live_abc', name: 'CI pipeline', scopes: ['read:billing'], created_at: '2026-01-02T00:00:00Z', last_used_at: '2026-02-03T00:00:00Z' },
          { id: 'k2', prefix: 'ak_live_xyz', name: 'Webhook worker', scopes: [], created_at: '2026-01-05T00:00:00Z', last_used_at: null },
        ]),
      ),
    )

    render(<ApiKeysCard />)

    expect(await screen.findByText('CI pipeline')).toBeInTheDocument()
    expect(screen.getByText('Webhook worker')).toBeInTheDocument()
    expect(screen.getByText('ak_live_abc')).toBeInTheDocument()
    expect(screen.getByText('read:billing')).toBeInTheDocument()
    // last_used_at null renders as "Never"
    expect(screen.getByText('Never')).toBeInTheDocument()
  })

  it('shows the empty state when there are no keys', async () => {
    server.use(http.get(KEYS_PATH, () => listResponse([])))

    render(<ApiKeysCard />)

    expect(await screen.findByText('No API keys yet')).toBeInTheDocument()
  })
})

describe('ApiKeysCard — create', () => {
  it('shows the one-time secret once and never again after the modal closes', async () => {
    const user = userEvent.setup()
    const secret = 'ak_live_abc.SUPERSECRETVALUE123'

    server.use(
      http.get(KEYS_PATH, () => listResponse([])),
      http.post(KEYS_PATH, async ({ request }) => {
        // Idempotency-Key must accompany the create.
        expect(request.headers.get('idempotency-key')).toBeTruthy()
        const body = (await request.json()) as { name: string; scopes: string[] }
        expect(body.name).toBe('Deploy bot')
        expect(body.scopes).toEqual(['read:billing', 'write:members'])
        return HttpResponse.json(
          { id: 'new-1', prefix: 'ak_live_abc', name: body.name, scopes: body.scopes, api_key: secret, created_at: '2026-03-01T00:00:00Z' },
          { status: 201 },
        )
      }),
    )

    render(<ApiKeysCard />)
    await screen.findByText('No API keys yet')

    await user.click(screen.getAllByRole('button', { name: /new api key/i })[0])
    await user.type(screen.getByLabelText(/name/i), 'Deploy bot')
    await user.type(screen.getByLabelText(/scopes/i), 'read:billing write:members')
    await user.click(screen.getByRole('button', { name: /create key/i }))

    // The secret is revealed exactly once.
    const secretField = (await screen.findByLabelText('API key secret')) as HTMLInputElement
    expect(secretField.value).toBe(secret)

    // Dismiss the reveal — the secret must be gone from the DOM.
    await user.click(screen.getByRole('button', { name: /done/i }))

    await waitFor(() => {
      expect(screen.queryByLabelText('API key secret')).not.toBeInTheDocument()
    })
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument()
    expect(screen.queryByText(secret)).not.toBeInTheDocument()
  })
})

describe('ApiKeysCard — revoke', () => {
  it('removes the row after the revoke is confirmed', async () => {
    const user = userEvent.setup()
    let deleted = false

    server.use(
      http.get(KEYS_PATH, () =>
        listResponse([{ id: 'k1', prefix: 'ak_live_abc', name: 'CI pipeline', scopes: [], created_at: '2026-01-02T00:00:00Z', last_used_at: null }]),
      ),
      http.delete(`${KEYS_PATH}/k1`, () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(<ApiKeysCard />)
    expect(await screen.findByText('CI pipeline')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /revoke ci pipeline/i }))

    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /^revoke$/i }))

    await waitFor(() => {
      expect(screen.queryByText('CI pipeline')).not.toBeInTheDocument()
    })
    expect(deleted).toBe(true)
    expect(screen.getByText('No API keys yet')).toBeInTheDocument()
  })
})
