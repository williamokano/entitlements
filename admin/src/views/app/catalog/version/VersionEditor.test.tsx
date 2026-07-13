import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import VersionEditor from './components/VersionEditor'

const BASE = 'http://api.test'
const PLAN_ID = '11111111-1111-1111-1111-111111111111'
const VID = '22222222-2222-2222-2222-222222222222'
const VERSION_PATH = `${BASE}/api/v1/catalog/versions/${VID}`
const PATCH_PATH = `${BASE}/api/v1/catalog/plans/${PLAN_ID}/versions/${VID}`
const PUBLISH_PATH = `${PATCH_PATH}/publish`

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: '', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
})

// The sweetalert2 confirm button carries .swal2-confirm regardless of its label.
const swalConfirm = () => document.querySelector<HTMLButtonElement>('.swal2-confirm')

const draftVersion = (status: 'draft' | 'published') => ({
  id: VID,
  plan_id: PLAN_ID,
  plan_key: 'pro',
  version: 1,
  status,
  currency: 'USD',
  prices: [{ cycle: 'monthly', amount_minor: 1999 }],
  trial: { enabled: false, days: 0, card_required: false },
  grace_days: 0,
  feature_grants: { 'seats.max': 5 },
})

describe('VersionEditor — published version is read-only', () => {
  it('disables every field and offers no Save or Publish', async () => {
    server.use(http.get(VERSION_PATH, () => HttpResponse.json(draftVersion('published'))))

    render(<VersionEditor planId={PLAN_ID} vid={VID} />)

    // The immutability notice appears once loaded.
    expect(await screen.findByText(/published and immutable/i)).toBeInTheDocument()

    // Fields are disabled (fieldset disabled cascades to descendants).
    expect(screen.getByLabelText(/currency/i)).toBeDisabled()
    expect(screen.getByLabelText(/monthly price/i)).toBeDisabled()
    expect(screen.getByLabelText('Feature value')).toBeDisabled()

    // No mutating affordances exist, so no PATCH/publish is possible.
    expect(screen.queryByRole('button', { name: /save draft/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^publish$/i })).not.toBeInTheDocument()
  })
})

describe('VersionEditor — publish requires the confirm', () => {
  it('does not POST publish until the confirm dialog is accepted', async () => {
    const user = userEvent.setup()
    let patched = false
    let published = false
    server.use(
      http.get(VERSION_PATH, () => HttpResponse.json(draftVersion('draft'))),
      http.patch(PATCH_PATH, () => {
        patched = true
        return HttpResponse.json(draftVersion('draft'))
      }),
      http.post(PUBLISH_PATH, () => {
        published = true
        return HttpResponse.json(draftVersion('published'))
      }),
    )

    render(<VersionEditor planId={PLAN_ID} vid={VID} />)

    await user.click(await screen.findByRole('button', { name: /^publish$/i }))

    // Confirm dialog is open; nothing has been sent yet.
    await waitFor(() => expect(swalConfirm()).not.toBeNull())
    expect(published).toBe(false)

    await user.click(swalConfirm() as HTMLButtonElement)

    await waitFor(() => expect(published).toBe(true))
    expect(patched).toBe(true)
  })
})
