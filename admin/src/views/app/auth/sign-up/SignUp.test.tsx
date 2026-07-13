import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import SignUpPage from './index'

const BASE = 'http://api.test'
const REGISTER_PATH = `${BASE}/api/v1/auth/register`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: 't1', appName: 'Test', enableDemo: false }
})

const renderSignUp = () =>
  render(
    <MemoryRouter initialEntries={['/auth/sign-up']}>
      <Routes>
        <Route path="/auth/sign-up" element={<SignUpPage />} />
        <Route path="/auth/sign-in" element={<div>sign in page</div>} />
      </Routes>
    </MemoryRouter>,
  )

describe('Sign-up client-side validation', () => {
  it('blocks an invalid email and a short password before any request', async () => {
    const user = userEvent.setup()
    let called = false
    server.use(
      http.post(REGISTER_PATH, () => {
        called = true
        return HttpResponse.json({ id: 'u1', email: 'x', status: 'active' }, { status: 201 })
      }),
    )

    renderSignUp()
    await user.type(screen.getByLabelText(/email address/i), 'not-an-email')
    await user.type(screen.getByLabelText(/^password/i), 'short')
    await user.type(screen.getByLabelText(/confirm password/i), 'short')
    await user.click(screen.getByRole('button', { name: /create account/i }))

    expect(await screen.findByText(/enter a valid email address/i)).toBeInTheDocument()
    expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument()
    // No network request was made.
    expect(called).toBe(false)
  })

  it('submits and redirects to sign-in when the form is valid', async () => {
    const user = userEvent.setup()
    server.use(
      http.post(REGISTER_PATH, async ({ request }) => {
        const body = (await request.json()) as { email: string; password: string }
        expect(body).toEqual({ email: 'new@example.com', password: 'sup3rsecret' })
        return HttpResponse.json({ id: 'u1', email: body.email, status: 'active' }, { status: 201 })
      }),
    )

    renderSignUp()
    await user.type(screen.getByLabelText(/email address/i), 'new@example.com')
    await user.type(screen.getByLabelText(/^password/i), 'sup3rsecret')
    await user.type(screen.getByLabelText(/confirm password/i), 'sup3rsecret')
    await user.click(screen.getByRole('button', { name: /create account/i }))

    expect(await screen.findByText('sign in page')).toBeInTheDocument()
  })
})
