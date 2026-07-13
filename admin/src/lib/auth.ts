/**
 * Auth store — session actions on top of the token store and api client.
 *
 * `login`/`logout` are the token-bearing actions; the rest of the auth journey
 * (register, password recovery, email verification, self-service password
 * change and session management) is wired here too so the F-003 screens talk to
 * the real API through one place. All endpoints live under /api/v1/auth and are
 * tenant-agnostic (auth is global — no tenant header required).
 */

import { apiFetch } from './api'
import { clearTokens, getRefreshToken, setTokens } from './tokens'

export { isAuthenticated, subscribe } from './tokens'

export type TokenResponse = {
  access_token: string
  refresh_token: string
  token_type: string
  access_expires_at: string
}

export type RegisteredUser = {
  id: string
  email: string
  status: string
}

export type AuthSession = {
  id: string
  issued_at: string
  expires_at: string
  current: boolean
}

// Last email the user authenticated with, kept so authed screens (e.g. the
// "resend verification email" action) have an address to work with without a
// dedicated profile endpoint.
const EMAIL_KEY = 'entitlements.email'

export function getSessionEmail(): string | null {
  try {
    return localStorage.getItem(EMAIL_KEY)
  } catch {
    return null
  }
}

function setSessionEmail(email: string): void {
  try {
    localStorage.setItem(EMAIL_KEY, email)
  } catch {
    // Storage unavailable — non-fatal.
  }
}

function clearSessionEmail(): void {
  try {
    localStorage.removeItem(EMAIL_KEY)
  } catch {
    // ignore
  }
}

/** Create a new account. Does not sign the user in (the backend returns the user, not tokens). */
export async function register(email: string, password: string): Promise<RegisteredUser> {
  return apiFetch<RegisteredUser>('/api/v1/auth/register', {
    method: 'POST',
    body: { email, password },
    auth: false,
  })
}

export async function login(email: string, password: string): Promise<void> {
  const pair = await apiFetch<TokenResponse>('/api/v1/auth/login', {
    method: 'POST',
    body: { email, password },
    auth: false,
  })
  setTokens({ accessToken: pair.access_token, refreshToken: pair.refresh_token })
  setSessionEmail(email)
}

export async function logout(): Promise<void> {
  const refreshToken = getRefreshToken()
  try {
    if (refreshToken) {
      await apiFetch('/api/v1/auth/logout', {
        method: 'POST',
        body: { refresh_token: refreshToken },
        auth: false,
      })
    }
  } catch {
    // Best effort — the server session may already be gone.
  } finally {
    clearTokens()
    clearSessionEmail()
  }
}

/** Request a password-reset email. Always resolves (no account enumeration). */
export async function requestPasswordReset(email: string): Promise<void> {
  await apiFetch('/api/v1/auth/password/forgot', {
    method: 'POST',
    body: { email },
    auth: false,
  })
}

/** Complete a password reset with the token from the emailed link. */
export async function resetPassword(token: string, password: string): Promise<void> {
  await apiFetch('/api/v1/auth/password/reset', {
    method: 'POST',
    body: { token, password },
    auth: false,
  })
}

/** Ask the backend to (re)send an email-verification link. Always resolves (no enumeration). */
export async function requestEmailVerification(email: string): Promise<void> {
  await apiFetch('/api/v1/auth/verify-email/request', {
    method: 'POST',
    body: { email },
    auth: false,
  })
}

/** Confirm an email address with the token from the emailed link. */
export async function verifyEmail(token: string): Promise<void> {
  await apiFetch('/api/v1/auth/verify-email', {
    method: 'POST',
    body: { token },
    auth: false,
  })
}

/** Change the signed-in user's password (revokes other sessions server-side). */
export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await apiFetch('/api/v1/auth/password/change', {
    method: 'POST',
    body: { current_password: currentPassword, new_password: newPassword },
  })
}

/** List the signed-in user's active sessions. */
export async function listSessions(): Promise<AuthSession[]> {
  const res = await apiFetch<{ sessions: AuthSession[] }>('/api/v1/auth/sessions')
  return res?.sessions ?? []
}

/** Revoke every session except the current one ("log out other devices"). */
export async function revokeOtherSessions(): Promise<void> {
  await apiFetch('/api/v1/auth/sessions/revoke-others', { method: 'POST' })
}
