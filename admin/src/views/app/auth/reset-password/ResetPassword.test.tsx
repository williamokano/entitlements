import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import ResetPasswordPage from './index'

const BASE = 'http://api.test'
const RESET_PATH = `${BASE}/api/v1/auth/password/reset`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = { apiBaseUrl: BASE, tenantMode: 'header', tenantSlug: 't1', appName: 'Test', enableDemo: false }
})

const renderReset = (entry: string) =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/auth/reset-password" element={<ResetPasswordPage />} />
        <Route path="/auth/sign-in" element={<div>sign in page</div>} />
      </Routes>
    </MemoryRouter>,
  )

describe('Reset password', () => {
  it('posts the token from the URL and routes to sign-in on success', async () => {
    const user = userEvent.setup()
    let received: { token: string; password: string } | null = null
    server.use(
      http.post(RESET_PATH, async ({ request }) => {
        received = (await request.json()) as { token: string; password: string }
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderReset('/auth/reset-password?token=reset-token-123')
    await user.type(screen.getByLabelText(/^new password/i), 'brand-new-pass')
    await user.type(screen.getByLabelText(/confirm new password/i), 'brand-new-pass')
    await user.click(screen.getByRole('button', { name: /update password/i }))

    expect(await screen.findByText('sign in page')).toBeInTheDocument()
    expect(received).toEqual({ token: 'reset-token-123', password: 'brand-new-pass' })
  })

  it('shows an error when the token is missing from the URL', () => {
    renderReset('/auth/reset-password')
    expect(screen.getByRole('alert')).toHaveTextContent(/missing its token/i)
  })
})
