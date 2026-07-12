/**
 * Access/refresh token storage — the single source of truth for the session.
 *
 * Kept separate from the api client and the auth actions so that both can use
 * it without an import cycle (api.ts needs tokens for headers/refresh, auth.ts
 * needs api.ts for login/logout).
 */

export type TokenPair = {
  accessToken: string
  refreshToken: string
}

const STORAGE_KEY = 'entitlements.tokens'

type Listener = () => void
const listeners = new Set<Listener>()

const readStorage = (): TokenPair | null => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<TokenPair>
    if (typeof parsed.accessToken !== 'string' || typeof parsed.refreshToken !== 'string') return null
    return { accessToken: parsed.accessToken, refreshToken: parsed.refreshToken }
  } catch {
    return null
  }
}

// In-memory snapshot (stable reference for useSyncExternalStore).
let current: TokenPair | null = readStorage()

const notify = () => listeners.forEach((listener) => listener())

export const getTokens = (): TokenPair | null => current

export const getAccessToken = (): string | null => current?.accessToken ?? null

export const getRefreshToken = (): string | null => current?.refreshToken ?? null

export const isAuthenticated = (): boolean => current !== null

export function setTokens(pair: TokenPair): void {
  current = pair
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(pair))
  } catch {
    // Storage unavailable (private mode, …) — session stays in memory.
  }
  notify()
}

export function clearTokens(): void {
  current = null
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
  notify()
}

/** Subscribe to session changes; returns an unsubscribe function. */
export function subscribe(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
