import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter, Route, Routes } from 'react-router'
import { ToastContainer } from 'react-toastify'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import type { CreditNote, Invoice } from './api'
import { formatMinor } from './helpers'
import Detail from './detail'
import List from './index'

const API = 'http://api.test'
const INVOICES = `${API}/api/v1/billing/invoices`

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: API, tenantMode: 'header', tenantSlug: 'tenant-1', appName: 'Test', enableDemo: false }
  setTokens({ accessToken: 'acc-1', refreshToken: 'ref-1' })
})

const makeInvoice = (over: Partial<Invoice> = {}): Invoice => ({
  id: 'inv-1',
  number: 42,
  subscription_id: 's-1',
  status: 'open',
  currency: 'USD',
  subtotal_minor: 2000,
  tax_minor: 0,
  total_minor: 2000,
  issued_at: '2026-07-01T00:00:00Z',
  line_items: [
    { kind: 'plan', description: 'Plan pro v1 (monthly)', key: 'pro', version: 1, unit_price_minor: 1500, quantity: 1, amount_minor: 1500, currency: 'USD' },
    { kind: 'addon', description: 'Addon seats v1 (monthly) x1', key: 'seats', version: 1, unit_price_minor: 500, quantity: 1, amount_minor: 500, currency: 'USD' },
  ],
  ...over,
})

const renderList = () =>
  render(
    <MemoryRouter>
      <List />
      <ToastContainer />
    </MemoryRouter>,
  )

const renderDetail = (id = 'inv-1') =>
  render(
    <MemoryRouter initialEntries={[`/billing/invoices/${id}`]}>
      <Routes>
        <Route path="/billing/invoices/:id" element={<Detail />} />
        <Route path="/billing" element={<div>Invoices list</div>} />
      </Routes>
      <ToastContainer />
    </MemoryRouter>,
  )

describe('Billing — invoice list', () => {
  it('renders invoice number, status, and total formatted from minor units', async () => {
    server.use(http.get(INVOICES, () => HttpResponse.json({ invoices: [makeInvoice({ total_minor: 1234 })] })))
    renderList()

    expect(await screen.findByText('INV-000042')).toBeInTheDocument()
    expect(screen.getByText('Open')).toBeInTheDocument()
    // Total is rendered from integer minor units (1234 → "$12.34"), never a float.
    expect(screen.getByText(formatMinor(1234, 'USD'))).toBeInTheDocument()
    expect(screen.getByText(formatMinor(1234, 'USD'))).toHaveTextContent('$12.34')
  })

  it('only shows the current tenant invoices (X-Tenant-ID isolation)', async () => {
    // The MSW handler mirrors the backend's tenant scoping: it reads the tenant
    // header the client attaches and returns only that tenant's invoices. Any
    // other tenant's invoice must never reach this client.
    const byTenant: Record<string, Invoice[]> = {
      'tenant-1': [makeInvoice({ id: 'inv-1', number: 1 })],
      'tenant-2': [makeInvoice({ id: 'inv-99', number: 99 })],
    }
    let seenTenant: string | null = null
    server.use(
      http.get(INVOICES, ({ request }) => {
        seenTenant = request.headers.get('X-Tenant-ID')
        return HttpResponse.json({ invoices: byTenant[seenTenant ?? ''] ?? [] })
      }),
    )
    renderList()

    expect(await screen.findByText('INV-000001')).toBeInTheDocument()
    expect(seenTenant).toBe('tenant-1')
    // Tenant 2's invoice is not shown.
    expect(screen.queryByText('INV-000099')).not.toBeInTheDocument()
  })
})

describe('Billing — invoice detail', () => {
  it('renders snapshotted line items and the subtotal / tax / total breakdown', async () => {
    server.use(
      http.get(`${INVOICES}/inv-1`, () => HttpResponse.json(makeInvoice({ subtotal_minor: 2000, tax_minor: 160, total_minor: 2160 }))),
      http.get(`${INVOICES}/inv-1/credit-notes`, () => HttpResponse.json({ credit_notes: [] })),
    )
    renderDetail()

    // Line item snapshot: description, key + version, unit price, quantity, amount.
    expect(await screen.findByText('Plan pro v1 (monthly)')).toBeInTheDocument()
    const planRow = screen.getByText('Plan pro v1 (monthly)').closest('tr') as HTMLElement
    expect(within(planRow).getByText('pro v1')).toBeInTheDocument()
    // The plan line's unit price + amount (1500 minor → "$15.00") render from minor units.
    expect(within(planRow).getAllByText(formatMinor(1500, 'USD')).length).toBeGreaterThan(0)
    expect(screen.getByText('seats v1')).toBeInTheDocument()

    // Totals breakdown, all from minor units.
    expect(screen.getByText('Subtotal').closest('tr')).toHaveTextContent(formatMinor(2000, 'USD'))
    expect(screen.getByText('Tax').closest('tr')).toHaveTextContent(formatMinor(160, 'USD'))
    expect(screen.getByText('Total').closest('tr')).toHaveTextContent(formatMinor(2160, 'USD'))
  })

  it('a lifecycle action (pay) updates the shown status', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(`${INVOICES}/inv-1`, () => HttpResponse.json(makeInvoice({ status: 'open' }))),
      http.get(`${INVOICES}/inv-1/credit-notes`, () => HttpResponse.json({ credit_notes: [] })),
      http.post(`${INVOICES}/inv-1/pay`, () => HttpResponse.json(makeInvoice({ status: 'paid' }))),
    )
    renderDetail()

    await user.click(await screen.findByRole('button', { name: 'Mark paid' }))

    // The status chip flips to Paid and the pay action disappears (terminal).
    expect(await screen.findByText('Paid')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Mark paid' })).not.toBeInTheDocument()
  })

  it('only shows transitions legal for the current status', async () => {
    server.use(
      http.get(`${INVOICES}/inv-1`, () => HttpResponse.json(makeInvoice({ status: 'paid' }))),
      http.get(`${INVOICES}/inv-1/credit-notes`, () => HttpResponse.json({ credit_notes: [] })),
    )
    renderDetail()

    // A paid invoice is terminal — no lifecycle transitions, but it can be credited.
    expect(await screen.findByText('Paid')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Mark paid' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Void' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Issue credit note' })).toBeInTheDocument()
  })
})

describe('Billing — credit notes', () => {
  it('blocks submitting a credit note with no reason', async () => {
    const user = userEvent.setup()
    let posted = false
    server.use(
      http.get(`${INVOICES}/inv-1`, () => HttpResponse.json(makeInvoice({ status: 'open' }))),
      http.get(`${INVOICES}/inv-1/credit-notes`, () => HttpResponse.json({ credit_notes: [] })),
      http.post(`${INVOICES}/inv-1/credit-notes`, () => {
        posted = true
        return HttpResponse.json({} as CreditNote, { status: 201 })
      }),
    )
    renderDetail()

    await user.click(await screen.findByRole('button', { name: 'Issue credit note' }))
    const dialog = screen.getByRole('dialog')
    // Provide an amount but leave the reason blank.
    await user.type(within(dialog).getByLabelText(/amount to credit/i), '500')
    await user.click(within(dialog).getByRole('button', { name: 'Issue credit note' }))

    // The yup schema blocks submit before any request is made.
    expect(await within(dialog).findByText('A reason is required')).toBeInTheDocument()
    expect(posted).toBe(false)
  })

  it('shows a created credit note with a negated amount', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(`${INVOICES}/inv-1`, () => HttpResponse.json(makeInvoice({ status: 'open' }))),
      http.get(`${INVOICES}/inv-1/credit-notes`, () => HttpResponse.json({ credit_notes: [] })),
      http.post(`${INVOICES}/inv-1/credit-notes`, async ({ request }) => {
        const body = (await request.json()) as { amount_minor: number; reason: string }
        // Backend stores the negation: a positive magnitude in, negative out.
        return HttpResponse.json(
          { id: 'cn-1', invoice_id: 'inv-1', number: 1, amount_minor: -body.amount_minor, currency: 'USD', reason: body.reason },
          { status: 201 },
        )
      }),
    )
    renderDetail()

    await user.click(await screen.findByRole('button', { name: 'Issue credit note' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText(/amount to credit/i), '500')
    await user.type(within(dialog).getByLabelText(/reason/i), 'Goodwill credit')
    await user.click(within(dialog).getByRole('button', { name: 'Issue credit note' }))

    // The credit note lists with a NEGATED amount (-$5.00 for a 500-minor credit).
    expect(await screen.findByText('CN-000001')).toBeInTheDocument()
    const negated = formatMinor(-500, 'USD')
    expect(negated).toBe('-$5.00')
    expect(screen.getByText(negated)).toBeInTheDocument()
  })
})
