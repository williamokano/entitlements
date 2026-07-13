import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import AddonVersionEditor from './components/AddonVersionEditor'

const BASE = 'http://api.test'
const ADDON_ID = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
const VID = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
const VERSION_PATH = `${BASE}/api/v1/catalog/addon-versions/${VID}`
const PLANS_PATH = `${BASE}/api/v1/catalog/plans`
const PATCH_PATH = `${BASE}/api/v1/catalog/addons/${ADDON_ID}/versions/${VID}`

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: '', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
})

const draftAddonVersion = {
  id: VID,
  addon_id: ADDON_ID,
  addon_key: 'boost',
  version: 1,
  status: 'draft',
  currency: 'USD',
  prices: [{ cycle: 'monthly', amount_minor: 500 }],
  quantity_allowed: false,
  compatible_plan_keys: [],
  deltas: [],
}

const baseHandlers = (onPatch: () => void) => [
  http.get(VERSION_PATH, () => HttpResponse.json(draftAddonVersion)),
  http.get(PLANS_PATH, () => HttpResponse.json({ plans: [{ id: 'p1', key: 'pro', name: 'Pro', status: 'active', public: true }] })),
  http.patch(PATCH_PATH, () => {
    onPatch()
    return HttpResponse.json(draftAddonVersion)
  }),
]

describe('AddonVersionEditor — delta validation (minor-units discipline)', () => {
  it('rejects an empty feature key before any PATCH', async () => {
    const user = userEvent.setup()
    let patched = false
    server.use(...baseHandlers(() => (patched = true)))

    render(<AddonVersionEditor addonId={ADDON_ID} vid={VID} />)
    await screen.findByRole('button', { name: /save draft/i })

    // Add a delta but leave the feature key empty.
    await user.click(screen.getByRole('button', { name: /add delta/i }))
    await user.click(screen.getByRole('button', { name: /save draft/i }))

    expect(await screen.findByText(/every delta needs a feature key/i)).toBeInTheDocument()
    expect(patched).toBe(false)
  })

  it('rejects a non-integer limit-delta amount before any PATCH', async () => {
    const user = userEvent.setup()
    let patched = false
    server.use(...baseHandlers(() => (patched = true)))

    render(<AddonVersionEditor addonId={ADDON_ID} vid={VID} />)
    await screen.findByRole('button', { name: /save draft/i })

    await user.click(screen.getByRole('button', { name: /add delta/i }))
    await user.type(screen.getByLabelText('Delta feature key'), 'api.calls')
    await user.type(screen.getByLabelText('Delta amount'), '1.5')
    await user.click(screen.getByRole('button', { name: /save draft/i }))

    expect(await screen.findByText(/amount must be a whole number/i)).toBeInTheDocument()
    expect(patched).toBe(false)
  })

  it('PATCHes a valid integer limit delta', async () => {
    const user = userEvent.setup()
    let patched = false
    let sentBody: unknown = null
    server.use(
      http.get(VERSION_PATH, () => HttpResponse.json(draftAddonVersion)),
      http.get(PLANS_PATH, () => HttpResponse.json({ plans: [] })),
      http.patch(PATCH_PATH, async ({ request }) => {
        patched = true
        sentBody = await request.json()
        return HttpResponse.json(draftAddonVersion)
      }),
    )

    render(<AddonVersionEditor addonId={ADDON_ID} vid={VID} />)
    await screen.findByRole('button', { name: /save draft/i })

    await user.click(screen.getByRole('button', { name: /add delta/i }))
    await user.type(screen.getByLabelText('Delta feature key'), 'api.calls')
    await user.type(screen.getByLabelText('Delta amount'), '1000')
    await user.click(screen.getByRole('button', { name: /save draft/i }))

    await waitFor(() => expect(patched).toBe(true))
    expect(sentBody).toMatchObject({ deltas: [{ feature_key: 'api.calls', kind: 'limit_delta', amount: 1000 }] })
  })
})
