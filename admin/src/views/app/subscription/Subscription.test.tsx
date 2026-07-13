import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter } from 'react-router'
import { ToastContainer } from 'react-toastify'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import Page from './index'
import type { Subscription } from './api'

const BASE = 'http://api.test'
const SUB = `${BASE}/api/v1/subscription`
const VERSION = `${BASE}/api/v1/catalog/versions/pv-1`
const CAT_ADDONS = `${BASE}/api/v1/catalog/addons`
const PUBLIC = `${BASE}/api/v1/catalog/public`

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: 'tenant-1', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
})

const PLAN_VERSION = {
  id: 'pv-1',
  plan_id: 'p-1',
  version: 1,
  status: 'published',
  currency: 'USD',
  prices: [{ cycle: 'monthly', amount_minor: 2000 }],
  trial: { enabled: false, days: 0, card_required: false },
  grace_days: 0,
  feature_grants: {},
  published_at: '2026-01-01T00:00:00Z',
  plan_key: 'pro',
}

const makeSub = (over: Partial<Subscription> = {}): Subscription => ({
  id: 's-1',
  plan_version_id: 'pv-1',
  billing_cycle: 'monthly',
  status: 'active',
  current_period_start: '2026-07-01T00:00:00Z',
  current_period_end: '2026-08-01T00:00:00Z',
  cancel_at_period_end: false,
  ...over,
})

const problem = (status: number, detail: string) =>
  HttpResponse.json({ status, title: 'Error', detail }, { status, headers: { 'content-type': 'application/problem+json' } })

// The load-time GETs a live subscription triggers: the subscription itself, its
// pinned plan version (display), and the addon catalog (attach picker). Addons
// default to empty so no per-addon GET fires.
const liveHandlers = (sub: Subscription, catalogAddons: unknown[] = []) => [
  http.get(SUB, () => HttpResponse.json(sub)),
  http.get(VERSION, () => HttpResponse.json(PLAN_VERSION)),
  http.get(CAT_ADDONS, () => HttpResponse.json({ addons: catalogAddons })),
]

const renderPage = () =>
  render(
    <MemoryRouter>
      <Page />
      <ToastContainer />
    </MemoryRouter>,
  )

describe('Subscription — lifecycle action gating', () => {
  it('an active subscription shows pause + cancel but not resume', async () => {
    server.use(...liveHandlers(makeSub({ status: 'active' })))
    renderPage()

    expect(await screen.findByRole('button', { name: 'Pause' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel subscription' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Reactivate' })).not.toBeInTheDocument()
  })

  it('a paused subscription shows resume (and not pause)', async () => {
    server.use(...liveHandlers(makeSub({ status: 'paused' })))
    renderPage()

    expect(await screen.findByRole('button', { name: 'Resume' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Pause' })).not.toBeInTheDocument()
  })

  it('a suspended subscription shows reactivate', async () => {
    server.use(...liveHandlers(makeSub({ status: 'suspended' })))
    renderPage()

    expect(await screen.findByRole('button', { name: 'Reactivate' })).toBeInTheDocument()
  })

  it('a canceled subscription shows no lifecycle actions', async () => {
    server.use(...liveHandlers(makeSub({ status: 'canceled' })))
    renderPage()

    // Wait for load (the status chip renders), then assert no action buttons.
    expect(await screen.findByText('Canceled')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Pause' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Cancel subscription' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Change plan' })).not.toBeInTheDocument()
  })
})

describe('Subscription — cancel modal', () => {
  it('posts immediate=false for an at-period-end cancel', async () => {
    const user = userEvent.setup()
    let body: { immediate?: boolean } = {}
    server.use(
      ...liveHandlers(makeSub({ status: 'active' })),
      http.post(`${SUB}/cancel`, async ({ request }) => {
        body = (await request.json()) as { immediate: boolean }
        return HttpResponse.json(makeSub({ cancel_at_period_end: true }))
      }),
    )
    renderPage()

    await user.click(await screen.findByRole('button', { name: 'Cancel subscription' }))
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel at period end' }))

    await waitFor(() => expect(body.immediate).toBe(false))
  })

  it('posts immediate=true when "Immediately" is chosen', async () => {
    const user = userEvent.setup()
    let body: { immediate?: boolean } = {}
    server.use(
      ...liveHandlers(makeSub({ status: 'active' })),
      http.post(`${SUB}/cancel`, async ({ request }) => {
        body = (await request.json()) as { immediate: boolean }
        return HttpResponse.json(makeSub({ status: 'canceled' }))
      }),
    )
    renderPage()

    await user.click(await screen.findByRole('button', { name: 'Cancel subscription' }))
    const dialog = screen.getByRole('dialog')
    // Second radio is "Immediately".
    await user.click(within(dialog).getAllByRole('radio')[1])
    await user.click(within(dialog).getByRole('button', { name: 'Cancel now' }))

    await waitFor(() => expect(body.immediate).toBe(true))
  })
})

describe('Subscription — conflict handling', () => {
  it('renders a conflict toast when a lifecycle action returns 409', async () => {
    const user = userEvent.setup()
    server.use(
      ...liveHandlers(makeSub({ status: 'active' })),
      http.post(`${SUB}/pause`, () => problem(409, 'cannot pause in this state')),
    )
    renderPage()

    await user.click(await screen.findByRole('button', { name: 'Pause' }))

    expect(await screen.findByText('cannot pause in this state')).toBeInTheDocument()
  })
})

describe('Subscription — empty state', () => {
  it('shows the plan picker when the tenant has no subscription (404)', async () => {
    server.use(
      http.get(SUB, () => problem(404, 'no subscription')),
      http.get(PUBLIC, () =>
        HttpResponse.json({
          plans: [
            {
              plan: { id: 'p-1', key: 'pro', name: 'Pro', status: 'active', public: true },
              version: { ...PLAN_VERSION, id: 'pv-1' },
            },
          ],
        }),
      ),
    )
    renderPage()

    expect(await screen.findByText('No active subscription')).toBeInTheDocument()
    // The picker renders the public plan with a Subscribe action.
    expect(await screen.findByRole('button', { name: 'Subscribe' })).toBeInTheDocument()
    expect(screen.getByText('Pro')).toBeInTheDocument()
  })
})

describe('Subscription — change plan (downgrade)', () => {
  it('renders the scheduled-change banner and cancels it', async () => {
    const user = userEvent.setup()
    let scheduledCancelCalled = false
    server.use(
      ...liveHandlers(makeSub({ status: 'active' })),
      http.get(PUBLIC, () =>
        HttpResponse.json({
          plans: [
            {
              plan: { id: 'p-2', key: 'starter', name: 'Starter', status: 'active', public: true },
              version: {
                id: 'pv-2',
                plan_id: 'p-2',
                version: 1,
                status: 'published',
                currency: 'USD',
                prices: [{ cycle: 'monthly', amount_minor: 1000 }],
                trial: { enabled: false, days: 0, card_required: false },
                grace_days: 0,
                feature_grants: {},
                published_at: null,
              },
            },
          ],
        }),
      ),
      http.post(`${SUB}/change-plan`, () => HttpResponse.json(makeSub({ scheduled_change: { plan_version_id: 'pv-2', billing_cycle: 'monthly' } }))),
      http.post(`${SUB}/scheduled-change/cancel`, () => {
        scheduledCancelCalled = true
        return HttpResponse.json(makeSub())
      }),
    )
    renderPage()

    // Open the change-plan picker and pick the downgrade plan.
    await user.click(await screen.findByRole('button', { name: 'Change plan' }))
    await user.click(await screen.findByRole('button', { name: 'Switch to this plan' }))

    // The downgrade schedules for period end — banner + cancel-change button appear.
    expect(await screen.findByText(/scheduled for the end of the current period/i)).toBeInTheDocument()
    const cancelChange = screen.getByRole('button', { name: 'Cancel change' })

    await user.click(cancelChange)
    await waitFor(() => expect(scheduledCancelCalled).toBe(true))
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Cancel change' })).not.toBeInTheDocument())
  })
})

describe('Subscription — addons', () => {
  const catalogAddon = { id: 'a1', key: 'seats', name: 'Extra Seats', status: 'active' }
  const addonDetail = {
    addon: catalogAddon,
    versions: [
      {
        id: 'av-1',
        addon_id: 'a1',
        version: 1,
        status: 'published',
        currency: 'USD',
        prices: [{ cycle: 'monthly', amount_minor: 500 }],
        quantity_allowed: true,
        compatible_plan_keys: ['pro'],
        deltas: [],
      },
    ],
  }

  it('validates the quantity before attaching', async () => {
    const user = userEvent.setup()
    let attachCalled = false
    server.use(
      ...liveHandlers(makeSub({ status: 'active' }), [catalogAddon]),
      http.get(`${CAT_ADDONS}/a1`, () => HttpResponse.json(addonDetail)),
      http.post(`${SUB}/addons`, () => {
        attachCalled = true
        return HttpResponse.json(makeSub())
      }),
    )
    renderPage()

    await user.selectOptions(await screen.findByLabelText('Addon'), 'av-1')
    const qty = screen.getByLabelText('Quantity') as HTMLInputElement
    // Empty (or fractional) quantity: passes native constraints but is rejected
    // by the component's own whole-number gate before any request is made.
    await user.clear(qty)
    await user.click(screen.getByRole('button', { name: 'Attach' }))

    expect(await screen.findByText(/quantity must be a whole number/i)).toBeInTheDocument()
    expect(attachCalled).toBe(false)
  })

  it('renders a 400 (incompatible) inline', async () => {
    const user = userEvent.setup()
    server.use(
      ...liveHandlers(makeSub({ status: 'active' }), [catalogAddon]),
      http.get(`${CAT_ADDONS}/a1`, () => HttpResponse.json(addonDetail)),
      http.post(`${SUB}/addons`, () => problem(400, 'addon is not compatible with the plan')),
    )
    renderPage()

    await user.selectOptions(await screen.findByLabelText('Addon'), 'av-1')
    await user.click(screen.getByRole('button', { name: 'Attach' }))

    expect(await screen.findByText('addon is not compatible with the plan')).toBeInTheDocument()
  })
})
