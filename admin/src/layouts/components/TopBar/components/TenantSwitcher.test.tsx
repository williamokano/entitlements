import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { apiFetch } from '@/lib/api'
import { forgetTenant, getCurrentTenantId, getKnownTenants, rememberAndSelect, rememberTenant } from '@/lib/tenant'
import { setTokens } from '@/lib/tokens'
import TenantSwitcher from './TenantSwitcher'

const BASE = 'http://api.test'
const PING_PATH = `${BASE}/api/v1/ping`

const A = '44444444-4444-4444-4444-444444444444'
const B = '55555555-5555-5555-5555-555555555555'

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const resetStore = () => getKnownTenants().slice().forEach((t) => forgetTenant(t.id))

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: '', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
  resetStore()
  rememberAndSelect({ id: A, name: 'Alpha', slug: 'alpha' })
  rememberTenant({ id: B, name: 'Beta', slug: 'beta' })
})

describe('TenantSwitcher', () => {
  it('selecting another tenant makes the next request carry its id as X-Tenant-ID', async () => {
    const user = userEvent.setup()
    server.use(http.get(PING_PATH, ({ request }) => HttpResponse.json({ tenant: request.headers.get('X-Tenant-ID') })))

    render(
      <MemoryRouter>
        <TenantSwitcher />
      </MemoryRouter>,
    )

    // Starts on Alpha.
    expect(getCurrentTenantId()).toBe(A)
    let res = await apiFetch<{ tenant: string }>('/api/v1/ping')
    expect(res.tenant).toBe(A)

    // Switch to Beta.
    await user.click(screen.getByRole('menuitemradio', { name: /Beta/ }))
    expect(getCurrentTenantId()).toBe(B)

    res = await apiFetch<{ tenant: string }>('/api/v1/ping')
    expect(res.tenant).toBe(B)
  })

  it('is hidden in subdomain tenant mode', () => {
    window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'subdomain', tenantSlug: '', appName: 'Test', enableDemo: false }
    const { container } = render(
      <MemoryRouter>
        <TenantSwitcher />
      </MemoryRouter>,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
