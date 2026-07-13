import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import SessionsCard from './components/SessionsCard'

const BASE = 'http://api.test'
const SESSIONS_PATH = `${BASE}/api/v1/auth/sessions`
const REVOKE_PATH = `${BASE}/api/v1/auth/sessions/revoke-others`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: 't1', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
})

const session = (id: string, current: boolean) => ({ id, issued_at: '2026-01-01T10:00:00Z', expires_at: '2026-02-01T10:00:00Z', current })

describe('SessionsCard', () => {
  it('lists sessions returned by GET', async () => {
    server.use(http.get(SESSIONS_PATH, () => HttpResponse.json({ sessions: [session('aaaaaaaa-1', true), session('bbbbbbbb-2', false)] })))

    render(<SessionsCard />)

    expect(await screen.findByText('aaaaaaaa')).toBeInTheDocument()
    expect(screen.getByText('bbbbbbbb')).toBeInTheDocument()
    expect(screen.getByText('This device')).toBeInTheDocument()
  })

  it('revokes other sessions and refreshes the list', async () => {
    const user = userEvent.setup()
    let revoked = false
    server.use(
      http.get(SESSIONS_PATH, () =>
        HttpResponse.json({ sessions: revoked ? [session('aaaaaaaa-1', true)] : [session('aaaaaaaa-1', true), session('bbbbbbbb-2', false)] }),
      ),
      http.post(REVOKE_PATH, () => {
        revoked = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(<SessionsCard />)
    expect(await screen.findByText('bbbbbbbb')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /log out other devices/i }))

    await waitFor(() => {
      expect(screen.queryByText('bbbbbbbb')).not.toBeInTheDocument()
    })
    expect(revoked).toBe(true)
    expect(screen.getByText('aaaaaaaa')).toBeInTheDocument()
  })
})
