/**
 * Auth store — session actions on top of the token store and api client.
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

export async function login(email: string, password: string): Promise<void> {
  const pair = await apiFetch<TokenResponse>('/api/v1/auth/login', {
    method: 'POST',
    body: { email, password },
    auth: false,
  })
  setTokens({ accessToken: pair.access_token, refreshToken: pair.refresh_token })
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
  }
}
