import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { clearTokens, getTokens } from '@/lib/tokens'
import SignInPage from './index'

const BASE = 'http://api.test'
const LOGIN_PATH = `${BASE}/api/v1/auth/login`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: 't1', appName: 'Test', enableDemo: false }
  clearTokens()
})

const renderSignIn = (entry: string | { pathname: string; state: unknown } = '/auth/sign-in') =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/auth/sign-in" element={<SignInPage />} />
        <Route path="/" element={<div>home page</div>} />
        <Route path="/invitations/:tenantId/:invId" element={<div>invitation page</div>} />
      </Routes>
    </MemoryRouter>,
  )

describe('Sign-in', () => {
  it('stores the token pair and redirects to / on success', async () => {
    const user = userEvent.setup()
    server.use(
      http.post(LOGIN_PATH, async ({ request }) => {
        const body = (await request.json()) as { email: string; password: string }
        expect(body).toEqual({ email: 'user@example.com', password: 'sup3rsecret' })
        return HttpResponse.json({ access_token: 'acc-1', refresh_token: 'ref-1', token_type: 'Bearer', access_expires_at: '2026-01-01T00:00:00Z' })
      }),
    )

    renderSignIn()
    await user.type(screen.getByLabelText(/email address/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'sup3rsecret')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('home page')).toBeInTheDocument()
    expect(getTokens()).toEqual({ accessToken: 'acc-1', refreshToken: 'ref-1' })
  })

  it('renders a 401 error inline and stays on the page', async () => {
    const user = userEvent.setup()
    server.use(
      http.post(LOGIN_PATH, () =>
        HttpResponse.json({ status: 401, title: 'Unauthorized', detail: 'Invalid email or password.' }, { status: 401, headers: { 'Content-Type': 'application/problem+json' } }),
      ),
    )

    renderSignIn()
    await user.type(screen.getByLabelText(/email address/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'wrong-password')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid email or password.')
    expect(screen.queryByText('home page')).not.toBeInTheDocument()
    expect(getTokens()).toBeNull()
  })

  // RequireAuth (and the invitation page) stash where the user was headed in
  // `state.from`; signing in must return them there, not to the app root.
  it('returns the user to where they were headed instead of the app root', async () => {
    const user = userEvent.setup()
    server.use(
      http.post(LOGIN_PATH, () =>
        HttpResponse.json({ access_token: 'acc-1', refresh_token: 'ref-1', token_type: 'Bearer', access_expires_at: '2026-01-01T00:00:00Z' }),
      ),
    )

    renderSignIn({ pathname: '/auth/sign-in', state: { from: { pathname: '/invitations/t-1/inv-1', search: '', hash: '' } } })
    await user.type(screen.getByLabelText(/email address/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'sup3rsecret')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('invitation page')).toBeInTheDocument()
    expect(screen.queryByText('home page')).not.toBeInTheDocument()
  })
})
