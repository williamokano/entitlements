import { render, screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import Pricing from './index'

const BASE = 'http://api.test'
const PUBLIC_PATH = `${BASE}/api/v1/catalog/public`

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: '', appName: 'Test', enableDemo: false }
})

const renderPage = () =>
  render(
    <MemoryRouter initialEntries={['/catalog/pricing']}>
      <Pricing />
    </MemoryRouter>,
  )

describe('Pricing preview', () => {
  it('renders the public plans returned by GET /catalog/public', async () => {
    server.use(
      http.get(PUBLIC_PATH, () =>
        HttpResponse.json({
          plans: [
            {
              plan: { id: 'p1', key: 'starter', name: 'Starter', status: 'active', public: true },
              version: {
                id: 'v1',
                plan_id: 'p1',
                version: 1,
                status: 'published',
                currency: 'USD',
                prices: [{ cycle: 'monthly', amount_minor: 1999 }],
                trial: { enabled: true, days: 14, card_required: false },
                grace_days: 0,
                feature_grants: { 'seats.max': 5 },
                published_at: '2026-01-01T00:00:00Z',
              },
            },
            {
              plan: { id: 'p2', key: 'pro', name: 'Pro', status: 'active', public: true },
              version: {
                id: 'v2',
                plan_id: 'p2',
                version: 3,
                status: 'published',
                currency: 'USD',
                prices: [{ cycle: 'annual', amount_minor: 199900 }],
                trial: { enabled: false, days: 0, card_required: false },
                grace_days: 0,
                feature_grants: {},
                published_at: '2026-01-01T00:00:00Z',
              },
            },
          ],
        }),
      ),
    )

    renderPage()

    expect(await screen.findByText('Starter')).toBeInTheDocument()
    expect(screen.getByText('Pro')).toBeInTheDocument()
    // Minor units formatted for display (1999 -> $19.99, 199900 -> $1,999.00).
    expect(screen.getByText('$19.99')).toBeInTheDocument()
    expect(screen.getByText('$1,999.00')).toBeInTheDocument()
    expect(screen.getByText(/14-day free trial/i)).toBeInTheDocument()
  })

  it('shows the empty state when no plans are public', async () => {
    server.use(http.get(PUBLIC_PATH, () => HttpResponse.json({ plans: [] })))

    renderPage()

    expect(await screen.findByText('No public plans')).toBeInTheDocument()
  })
})
