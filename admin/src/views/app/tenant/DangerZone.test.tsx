import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { ToastContainer } from 'react-toastify'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { forgetTenant, getKnownTenants, rememberAndSelect } from '@/lib/tenant'
import { setTokens } from '@/lib/tokens'
import DangerZone from './components/DangerZone'
import type { Tenant } from './api'

const BASE = 'http://api.test'
const TENANT_ID = '33333333-3333-3333-3333-333333333333'
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

const tenant: Tenant = { id: TENANT_ID, name: 'Acme', slug: 'acme', status: 'active' }

// The sweetalert2 confirm button carries the .swal2-confirm class regardless of
// its label; querying it directly avoids clashing with the page's own buttons.
const swalConfirm = () => document.querySelector<HTMLButtonElement>('.swal2-confirm')

describe('Tenant danger zone — delete', () => {
  it('does not fire DELETE until the confirm dialog is accepted', async () => {
    const user = userEvent.setup()
    let deleted = false
    server.use(
      http.delete(TENANT_PATH, () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(
      <>
        <DangerZone tenant={tenant} />
        <ToastContainer />
      </>,
    )

    // Open the confirm dialog.
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(swalConfirm()).not.toBeNull())

    // Still not deleted — only the dialog is open.
    expect(deleted).toBe(false)

    // Accept the confirmation.
    await user.click(swalConfirm() as HTMLButtonElement)
    await waitFor(() => expect(deleted).toBe(true))
  })

  it('does not fire DELETE when the confirm dialog is dismissed', async () => {
    const user = userEvent.setup()
    let deleted = false
    server.use(
      http.delete(TENANT_PATH, () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(<DangerZone tenant={tenant} />)

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(document.querySelector('.swal2-cancel')).not.toBeNull())
    await user.click(document.querySelector('.swal2-cancel') as HTMLButtonElement)

    await waitFor(() => expect(document.querySelector('.swal2-cancel')).toBeNull())
    expect(deleted).toBe(false)
  })
})
