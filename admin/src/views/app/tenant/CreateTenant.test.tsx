import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { apiFetch } from '@/lib/api'
import { forgetTenant, getKnownTenants } from '@/lib/tenant'
import { setTokens } from '@/lib/tokens'
import Onboarding from './onboarding'

const BASE = 'http://api.test'
const TENANTS_PATH = `${BASE}/api/v1/tenants`
const PING_PATH = `${BASE}/api/v1/ping`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const resetStore = () => getKnownTenants().slice().forEach((t) => forgetTenant(t.id))

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: '', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
  resetStore()
})

// A stand-in dashboard that fires a request on mount and surfaces the tenant
// header the api client attached — proving the new tenant is now current.
const HeaderProbe = () => {
  const [header, setHeader] = useState<string | null>(null)
  useEffect(() => {
    void apiFetch<{ tenant: string | null }>('/api/v1/ping').then((res) => setHeader(res.tenant))
  }, [])
  return <div>dashboard tenant: {header ?? '—'}</div>
}

describe('Tenant onboarding', () => {
  it('creates a tenant, routes to the dashboard, and sends its id as X-Tenant-ID on the next request', async () => {
    const user = userEvent.setup()
    const NEW_ID = '11111111-1111-1111-1111-111111111111'

    server.use(
      http.post(TENANTS_PATH, async ({ request }) => {
        const body = (await request.json()) as { name: string; slug: string }
        expect(body.name).toBe('Acme Inc.')
        expect(body.slug).toBe('acme-inc')
        return HttpResponse.json({ id: NEW_ID, name: body.name, slug: body.slug, status: 'active' }, { status: 201 })
      }),
      http.get(PING_PATH, ({ request }) => HttpResponse.json({ tenant: request.headers.get('X-Tenant-ID') })),
    )

    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route path="/" element={<Onboarding />} />
          <Route path="/dashboard" element={<HeaderProbe />} />
        </Routes>
      </MemoryRouter>,
    )

    await user.type(screen.getByLabelText(/tenant name/i), 'Acme Inc.')
    await user.click(screen.getByRole('button', { name: /create tenant & continue/i }))

    // Routed to the dashboard, and the follow-up request carried the new id.
    expect(await screen.findByText(`dashboard tenant: ${NEW_ID}`)).toBeInTheDocument()
  })
})
