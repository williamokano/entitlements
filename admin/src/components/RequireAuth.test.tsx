import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { beforeEach, describe, expect, it } from 'vitest'
import { clearTokens, setTokens } from '@/lib/tokens'
import RequireAuth from './RequireAuth'

const renderProtected = () =>
  render(
    <MemoryRouter initialEntries={['/protected']}>
      <Routes>
        <Route path="/auth/sign-in" element={<div>sign-in page</div>} />
        <Route
          path="/protected"
          element={
            <RequireAuth>
              <div>secret content</div>
            </RequireAuth>
          }
        />
      </Routes>
    </MemoryRouter>,
  )

describe('RequireAuth', () => {
  beforeEach(() => {
    clearTokens()
  })

  it('redirects anonymous users to /auth/sign-in', () => {
    renderProtected()

    expect(screen.getByText('sign-in page')).toBeInTheDocument()
    expect(screen.queryByText('secret content')).not.toBeInTheDocument()
  })

  it('renders children when authenticated', () => {
    setTokens({ accessToken: 'access', refreshToken: 'refresh' })

    renderProtected()

    expect(screen.getByText('secret content')).toBeInTheDocument()
    expect(screen.queryByText('sign-in page')).not.toBeInTheDocument()
  })
})
