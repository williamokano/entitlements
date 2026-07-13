import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import Detail from './detail'
import List from './index'

const API = 'http://api.test'
const ENTITLEMENTS = `${API}/api/v1/entitlements`

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: API, tenantMode: 'header', tenantSlug: 'tenant-1', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
})

// A representative effective set spanning every value type and source layer, in
// the exact map-under-"entitlements" shape the backend's getAll handler returns.
const fullSet = {
  entitlements: {
    sso: { value: true, source: 'plan' },
    // A limit lifted by an add-on (base + delta) — winning layer is the add-on.
    seats: { value: 25, source: 'addon' },
    // A config value forced by a manual, time-bound override.
    theme: { value: 'dark', source: 'override', expires_at: '2026-12-31T00:00:00Z' },
    // An enum still at its catalog default (no subscription touched it).
    tier: { value: 'silver', source: 'default' },
  },
}

const renderList = () =>
  render(
    <MemoryRouter initialEntries={['/entitlements']}>
      <Routes>
        <Route path="/entitlements" element={<List />} />
        <Route path="/entitlements/:key" element={<div>Drill-in for feature</div>} />
      </Routes>
    </MemoryRouter>,
  )

const renderDetail = (key = 'seats') =>
  render(
    <MemoryRouter initialEntries={[`/entitlements/${key}`]}>
      <Routes>
        <Route path="/entitlements/:key" element={<Detail />} />
        <Route path="/entitlements" element={<div>Entitlements list</div>} />
      </Routes>
    </MemoryRouter>,
  )

describe('Entitlements — effective set table', () => {
  it('renders the whole set from a SINGLE GET /entitlements call', async () => {
    let calls = 0
    server.use(
      http.get(ENTITLEMENTS, () => {
        calls += 1
        return HttpResponse.json(fullSet)
      }),
    )
    renderList()

    // Every feature key from the one call is present.
    expect(await screen.findByRole('button', { name: 'Open sso' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open seats' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open theme' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open tier' })).toBeInTheDocument()
    // Exactly one request fed the whole table.
    expect(calls).toBe(1)
  })

  it('renders each value by its inferred type: boolean chip, limit number, config/enum value', async () => {
    server.use(http.get(ENTITLEMENTS, () => HttpResponse.json(fullSet)))
    renderList()

    const ssoRow = (await screen.findByRole('button', { name: 'Open sso' })).closest('tr') as HTMLElement
    // Boolean → an on/off chip.
    expect(within(ssoRow).getByText('On')).toBeInTheDocument()

    const seatsRow = screen.getByRole('button', { name: 'Open seats' }).closest('tr') as HTMLElement
    // Limit → the number.
    expect(within(seatsRow).getByText('25')).toBeInTheDocument()

    const themeRow = screen.getByRole('button', { name: 'Open theme' }).closest('tr') as HTMLElement
    // Config/enum → the value verbatim.
    expect(within(themeRow).getByText('dark')).toBeInTheDocument()
  })

  it('the source badge on each row reflects the winning layer', async () => {
    server.use(http.get(ENTITLEMENTS, () => HttpResponse.json(fullSet)))
    renderList()

    const seatsRow = (await screen.findByRole('button', { name: 'Open seats' })).closest('tr') as HTMLElement
    // seats was lifted by an add-on → Add-on badge, not Plan/Default.
    expect(within(seatsRow).getByText('Add-on')).toBeInTheDocument()

    const themeRow = screen.getByRole('button', { name: 'Open theme' }).closest('tr') as HTMLElement
    expect(within(themeRow).getByText('Override')).toBeInTheDocument()

    const ssoRow = screen.getByRole('button', { name: 'Open sso' }).closest('tr') as HTMLElement
    expect(within(ssoRow).getByText('Plan')).toBeInTheDocument()
  })

  it('an empty / no-subscription tenant shows only default-sourced rows', async () => {
    // No subscription: the resolver still emits every feature at its catalog
    // default, so every row carries the Default badge and nothing else.
    server.use(
      http.get(ENTITLEMENTS, () =>
        HttpResponse.json({
          entitlements: {
            sso: { value: false, source: 'default' },
            seats: { value: 10, source: 'default' },
          },
        }),
      ),
    )
    renderList()

    expect(await screen.findByRole('button', { name: 'Open sso' })).toBeInTheDocument()
    // Both rows are Default-sourced; no plan/addon/override badge appears.
    expect(screen.getAllByText('Default')).toHaveLength(2)
    expect(screen.queryByText('Plan')).not.toBeInTheDocument()
    expect(screen.queryByText('Add-on')).not.toBeInTheDocument()
    expect(screen.queryByText('Override')).not.toBeInTheDocument()
  })
})

describe('Entitlements — per-feature drill-in', () => {
  it('reuses GET /entitlements/{key} to show the effective value, source, and expiry', async () => {
    let seenKey: string | null = null
    server.use(
      http.get(`${ENTITLEMENTS}/:key`, ({ params }) => {
        seenKey = params.key as string
        return HttpResponse.json({ key: 'theme', value: 'dark', source: 'override', expires_at: '2026-12-31T00:00:00Z' })
      }),
    )
    renderDetail('theme')

    // The drill-in fetched the single feature by key.
    expect(await screen.findByText('Feature')).toBeInTheDocument()
    expect(seenKey).toBe('theme')
    // Value, source badge, and time-bound expiry are all shown.
    expect(screen.getByText('dark')).toBeInTheDocument()
    expect(screen.getByText('Override')).toBeInTheDocument()
    expect(screen.getByText('Expires')).toBeInTheDocument()
  })

  it('navigating from a table row opens that feature drill-in', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(ENTITLEMENTS, () => HttpResponse.json(fullSet)),
      http.get(`${ENTITLEMENTS}/seats`, () => HttpResponse.json({ key: 'seats', value: 25, source: 'addon' })),
    )
    render(
      <MemoryRouter initialEntries={['/entitlements']}>
        <Routes>
          <Route path="/entitlements" element={<List />} />
          <Route path="/entitlements/:key" element={<Detail />} />
        </Routes>
      </MemoryRouter>,
    )

    await user.click(await screen.findByRole('button', { name: 'Open seats' }))

    // The drill-in for that feature renders its value and winning layer.
    expect(await screen.findByText('Feature')).toBeInTheDocument()
    expect(screen.getByText('25')).toBeInTheDocument()
    expect(screen.getByText('Add-on')).toBeInTheDocument()
  })
})
