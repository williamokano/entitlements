import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { ToastContainer } from 'react-toastify'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { forgetTenant, getKnownTenants, rememberAndSelect } from '@/lib/tenant'
import { setTokens } from '@/lib/tokens'
import TenantSettingsForm from './components/TenantSettingsForm'

const BASE = 'http://api.test'
const TENANT_ID = '22222222-2222-2222-2222-222222222222'
const TENANT_PATH = `${BASE}/api/v1/tenants/${TENANT_ID}`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const resetStore = () => getKnownTenants().slice().forEach((t) => forgetTenant(t.id))

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: '', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
  resetStore()
  rememberAndSelect({ id: TENANT_ID, name: 'Acme', slug: 'acme' })
})

describe('Tenant settings', () => {
  it('loads the tenant and round-trips a PATCH with a success toast', async () => {
    const user = userEvent.setup()
    let patched: { name?: string; settings?: unknown } | null = null

    server.use(
      http.get(TENANT_PATH, () => HttpResponse.json({ id: TENANT_ID, name: 'Acme', slug: 'acme', status: 'active' })),
      http.patch(TENANT_PATH, async ({ request }) => {
        patched = (await request.json()) as { name?: string; settings?: unknown }
        return HttpResponse.json({ id: TENANT_ID, name: patched.name, slug: 'acme', status: 'active' })
      }),
    )

    render(
      <>
        <TenantSettingsForm tenantId={TENANT_ID} />
        <ToastContainer />
      </>,
    )

    // Loaded value populates the field.
    const nameInput = await screen.findByLabelText(/tenant name/i)
    expect(nameInput).toHaveValue('Acme')

    await user.clear(nameInput)
    await user.type(nameInput, 'Acme Corp')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    expect(await screen.findByText(/tenant settings saved/i)).toBeInTheDocument()
    expect(patched).toEqual({ name: 'Acme Corp', settings: {} })
  })
})
