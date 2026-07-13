import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { setTokens } from '@/lib/tokens'
import InvitationsCard from './components/InvitationsCard'
import MembersCard from './components/MembersCard'

const BASE = 'http://api.test'
const TENANT = '11111111-1111-1111-1111-111111111111'
const MEMBERS_PATH = `${BASE}/api/v1/tenants/${TENANT}/members`
const INVITATIONS_PATH = `${BASE}/api/v1/tenants/${TENANT}/invitations`
const ROLES_PATH = `${BASE}/api/v1/roles`

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  window.__APP_CONFIG__ = {
    apiBaseUrl: BASE,
    tenantMode: 'header',
    tenantSlug: TENANT,
    appName: 'Test',
    enableDemo: false,
  }
  setTokens({ accessToken: 'access-1', refreshToken: 'refresh-1' })
})

const member = (overrides: Partial<Record<string, unknown>> = {}) => ({
  user_id: '22222222-2222-2222-2222-222222222222',
  email: 'ada@example.com',
  role: 'member',
  status: 'active',
  ...overrides,
})

const rolesResponse = () =>
  HttpResponse.json({
    roles: [
      { id: 'r1', name: 'owner', system: true },
      { id: 'r2', name: 'admin', system: true },
      { id: 'r3', name: 'member', system: true },
      { id: 'r4', name: 'billing-viewer', system: false },
    ],
  })

describe('MembersCard', () => {
  it('renders the members returned by GET, by email', async () => {
    server.use(
      http.get(MEMBERS_PATH, () =>
        HttpResponse.json({
          members: [member(), member({ user_id: '33333333-3333-3333-3333-333333333333', email: 'grace@example.com', role: 'owner' })],
        }),
      ),
    )

    render(<MembersCard tenantId={TENANT} />)

    expect(await screen.findByText('ada@example.com')).toBeInTheDocument()
    expect(screen.getByText('grace@example.com')).toBeInTheDocument()
    expect(screen.getByText('owner')).toBeInTheDocument()
  })

  // Memberships written before the backend stored the invited email have none.
  it('falls back to the user id when a member has no email', async () => {
    server.use(http.get(MEMBERS_PATH, () => HttpResponse.json({ members: [member({ email: '' })] })))

    render(<MembersCard tenantId={TENANT} />)

    expect(await screen.findByText('22222222-2222-2222-2222-222222222222')).toBeInTheDocument()
  })

  it('shows the empty state when the tenant has no members', async () => {
    server.use(http.get(MEMBERS_PATH, () => HttpResponse.json({ members: [] })))

    render(<MembersCard tenantId={TENANT} />)

    expect(await screen.findByText('No members yet')).toBeInTheDocument()
  })

  it('confirms before removing, and does not call DELETE when the confirm is dismissed', async () => {
    const user = userEvent.setup()
    let deleteCalls = 0
    server.use(
      http.get(MEMBERS_PATH, () => HttpResponse.json({ members: [member()] })),
      http.delete(`${MEMBERS_PATH}/:userId`, () => {
        deleteCalls += 1
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(<MembersCard tenantId={TENANT} />)
    await screen.findByText('ada@example.com')

    await user.click(screen.getByRole('button', { name: 'Remove ada@example.com' }))

    // sweetalert2 confirmation — dismiss it.
    const cancel = await screen.findByRole('button', { name: 'Cancel' })
    await user.click(cancel)

    await waitFor(() => expect(screen.queryByRole('button', { name: 'Cancel' })).not.toBeInTheDocument())
    expect(deleteCalls).toBe(0)
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
  })

  it('removes the row after the confirm is accepted', async () => {
    const user = userEvent.setup()
    let deletedUserId: string | undefined
    server.use(
      http.get(MEMBERS_PATH, () => HttpResponse.json({ members: [member()] })),
      http.delete(`${MEMBERS_PATH}/:userId`, ({ params }) => {
        deletedUserId = params.userId as string
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(<MembersCard tenantId={TENANT} />)
    await screen.findByText('ada@example.com')

    await user.click(screen.getByRole('button', { name: 'Remove ada@example.com' }))
    await user.click(await screen.findByRole('button', { name: 'Remove' }))

    await waitFor(() => expect(screen.queryByText('ada@example.com')).not.toBeInTheDocument())
    expect(deletedUserId).toBe('22222222-2222-2222-2222-222222222222')
    expect(await screen.findByText('No members yet')).toBeInTheDocument()
  })
})

describe('InvitationsCard', () => {
  const invitation = (overrides: Partial<Record<string, unknown>> = {}) => ({
    id: 'inv-1',
    email: 'newbie@example.com',
    role: 'member',
    status: 'pending',
    created_at: '2026-07-01T00:00:00Z',
    expires_at: '2026-07-08T00:00:00Z',
    ...overrides,
  })

  it('lists pending invitations', async () => {
    server.use(http.get(INVITATIONS_PATH, () => HttpResponse.json({ invitations: [invitation()] })))

    render(<InvitationsCard tenantId={TENANT} />)

    expect(await screen.findByText('newbie@example.com')).toBeInTheDocument()
  })

  it('resends an invitation and reflects the new expiry', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(INVITATIONS_PATH, () => HttpResponse.json({ invitations: [invitation()] })),
      http.post(`${INVITATIONS_PATH}/:invId/resend`, () => HttpResponse.json(invitation({ expires_at: '2026-12-25T00:00:00Z' }))),
    )

    render(<InvitationsCard tenantId={TENANT} />)
    await screen.findByText('newbie@example.com')

    await user.click(screen.getByRole('button', { name: 'Resend invitation to newbie@example.com' }))

    expect(await screen.findByText('Invitation resent to newbie@example.com.')).toBeInTheDocument()
    expect(screen.getByText(/Dec 25, 2026|25 Dec 2026/)).toBeInTheDocument()
  })

  it('rejects a malformed email before calling the API, then posts a valid one', async () => {
    const user = userEvent.setup()
    let posted: unknown = null
    server.use(
      http.get(INVITATIONS_PATH, () => HttpResponse.json({ invitations: [] })),
      http.get(ROLES_PATH, () => rolesResponse()),
      http.post(INVITATIONS_PATH, async ({ request }) => {
        posted = await request.json()
        return HttpResponse.json(invitation({ email: 'valid@example.com', role: 'admin' }), { status: 201 })
      }),
    )

    render(<InvitationsCard tenantId={TENANT} />)
    await screen.findByText('No pending invitations')

    await user.click(screen.getByRole('button', { name: /Invite member/ }))
    const dialog = await screen.findByRole('dialog')

    await user.type(within(dialog).getByLabelText('Email'), 'not-an-email')
    await user.click(within(dialog).getByRole('button', { name: 'Send invitation' }))

    expect(await within(dialog).findByText('Enter a valid email address.')).toBeInTheDocument()
    expect(posted).toBeNull()

    // Fix the email, pick a role, and submit for real.
    await user.clear(within(dialog).getByLabelText('Email'))
    await user.type(within(dialog).getByLabelText('Email'), 'valid@example.com')
    await user.selectOptions(within(dialog).getByLabelText('Role'), 'admin')
    await user.click(within(dialog).getByRole('button', { name: 'Send invitation' }))

    await waitFor(() => expect(posted).toEqual({ email: 'valid@example.com', role: 'admin' }))
    expect(await screen.findByText('valid@example.com')).toBeInTheDocument()
  })

  it('offers the tenant custom roles, and falls back to the system roles when roles are unreadable', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(INVITATIONS_PATH, () => HttpResponse.json({ invitations: [] })),
      http.get(ROLES_PATH, () => rolesResponse()),
    )

    const { unmount } = render(<InvitationsCard tenantId={TENANT} />)
    await screen.findByText('No pending invitations')
    await user.click(screen.getByRole('button', { name: /Invite member/ }))

    const roleSelect = await screen.findByLabelText('Role')
    await waitFor(() => expect(within(roleSelect).getByRole('option', { name: 'billing-viewer' })).toBeInTheDocument())
    unmount()

    // A caller without role:read gets a 403 — the form must still be usable.
    server.use(http.get(ROLES_PATH, () => new HttpResponse(null, { status: 403 })))

    render(<InvitationsCard tenantId={TENANT} />)
    await screen.findByText('No pending invitations')
    await user.click(screen.getByRole('button', { name: /Invite member/ }))

    const fallback = await screen.findByLabelText('Role')
    expect(within(fallback).getByRole('option', { name: 'owner' })).toBeInTheDocument()
    expect(within(fallback).getByRole('option', { name: 'member' })).toBeInTheDocument()
    expect(within(fallback).queryByRole('option', { name: 'billing-viewer' })).not.toBeInTheDocument()
  })

  it('renders a duplicate-invite conflict inline', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(INVITATIONS_PATH, () => HttpResponse.json({ invitations: [] })),
      http.get(ROLES_PATH, () => rolesResponse()),
      http.post(INVITATIONS_PATH, () =>
        HttpResponse.json(
          { status: 409, title: 'Conflict', detail: 'a pending invitation for this email already exists' },
          { status: 409, headers: { 'content-type': 'application/problem+json' } },
        ),
      ),
    )

    render(<InvitationsCard tenantId={TENANT} />)
    await screen.findByText('No pending invitations')
    await user.click(screen.getByRole('button', { name: /Invite member/ }))

    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Email'), 'dup@example.com')
    await user.click(within(dialog).getByRole('button', { name: 'Send invitation' }))

    expect(await within(dialog).findByText('a pending invitation for this email already exists')).toBeInTheDocument()
  })
})
