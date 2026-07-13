import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router'
import { clearTokens, setTokens } from '@/lib/tokens'
import { getCurrentTenantId } from '@/lib/tenant'
import AcceptInvitation from './index'

const BASE = 'http://api.test'
const TENANT = '11111111-1111-1111-1111-111111111111'
const INVITATION = '44444444-4444-4444-4444-444444444444'
const ACCEPT_PATH = `${BASE}/api/v1/tenants/${TENANT}/invitations/${INVITATION}/accept`
const DECLINE_PATH = `${BASE}/api/v1/tenants/${TENANT}/invitations/${INVITATION}/decline`
const TENANT_PATH = `${BASE}/api/v1/tenants/${TENANT}`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = {
    apiBaseUrl: BASE,
    tenantMode: 'header',
    tenantSlug: TENANT,
    appName: 'Test',
    enableDemo: false,
  }
})

// Renders the invitation route alongside stand-ins for the two places it can
// send the user, so we can assert where it routed to.
const renderAt = (path = `/invitations/${TENANT}/${INVITATION}`) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/invitations/:tenantId/:invId" element={<AcceptInvitation />} />
        <Route path="/auth/sign-in" element={<h1>Sign in page</h1>} />
        <Route path="/" element={<h1>The app</h1>} />
      </Routes>
    </MemoryRouter>,
  )

const problem = (status: number, detail: string) =>
  HttpResponse.json({ status, title: 'Error', detail }, { status, headers: { 'content-type': 'application/problem+json' } })

describe('invitation accept page — anonymous', () => {
  beforeEach(() => clearTokens())

  it('sends an anonymous invitee to sign in rather than calling the API', async () => {
    const user = userEvent.setup()
    renderAt()

    expect(await screen.findByText('You have been invited')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Sign in to accept' }))

    expect(await screen.findByText('Sign in page')).toBeInTheDocument()
  })
})

describe('invitation accept page — signed in', () => {
  beforeEach(() => setTokens({ accessToken: 'access-1', refreshToken: 'refresh-1' }))

  it('accepts the invitation and routes onward into the app', async () => {
    const user = userEvent.setup()
    let accepted = false
    server.use(
      http.post(ACCEPT_PATH, () => {
        accepted = true
        return HttpResponse.json({ user_id: 'u1', email: 'ada@example.com', role: 'member', status: 'active' }, { status: 201 })
      }),
      http.get(TENANT_PATH, () => HttpResponse.json({ id: TENANT, name: 'Acme', slug: 'acme', status: 'active' })),
    )

    renderAt()
    await user.click(await screen.findByRole('button', { name: 'Accept invitation' }))

    expect(await screen.findByText('The app')).toBeInTheDocument()
    expect(accepted).toBe(true)
    // The invitee lands in the organization they just joined.
    await waitFor(() => expect(getCurrentTenantId()).toBe(TENANT))
  })

  it('declines the invitation and ends in a terminal state', async () => {
    const user = userEvent.setup()
    let declined = false
    server.use(
      http.post(DECLINE_PATH, () => {
        declined = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderAt()
    await user.click(await screen.findByRole('button', { name: 'Decline' }))

    expect(await screen.findByText('Invitation declined')).toBeInTheDocument()
    expect(declined).toBe(true)
  })

  it('explains a 403 (the invitation belongs to another email) instead of crashing', async () => {
    const user = userEvent.setup()
    server.use(http.post(ACCEPT_PATH, () => problem(403, 'invitation is not for this user')))

    renderAt()
    await user.click(await screen.findByRole('button', { name: 'Accept invitation' }))

    expect(await screen.findByText('This invitation is for someone else')).toBeInTheDocument()
    expect(screen.queryByText('The app')).not.toBeInTheDocument()
  })

  it('explains a 409 (expired or already used)', async () => {
    const user = userEvent.setup()
    server.use(http.post(ACCEPT_PATH, () => problem(409, 'invitation is no longer acceptable')))

    renderAt()
    await user.click(await screen.findByRole('button', { name: 'Accept invitation' }))

    expect(await screen.findByText('This invitation is no longer valid')).toBeInTheDocument()
  })
})
