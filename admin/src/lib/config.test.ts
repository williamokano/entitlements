import { afterEach, describe, expect, it, vi } from 'vitest'
import { appConfig } from './config'

afterEach(() => {
  delete window.__APP_CONFIG__
  vi.unstubAllEnvs()
})

describe('appConfig', () => {
  it('prefers window.__APP_CONFIG__ over VITE_* env', () => {
    window.__APP_CONFIG__ = {
      apiBaseUrl: 'https://api.example.com/',
      tenantMode: 'subdomain',
      tenantSlug: 'acme',
      appName: 'Acme Corp',
      enableDemo: false,
    }
    vi.stubEnv('VITE_API_BASE_URL', 'http://from-env:9999')
    vi.stubEnv('VITE_APP_NAME', 'EnvName')

    const cfg = appConfig()
    expect(cfg.apiBaseUrl).toBe('https://api.example.com') // trailing slash stripped
    expect(cfg.tenantMode).toBe('subdomain')
    expect(cfg.tenantSlug).toBe('acme')
    expect(cfg.appName).toBe('Acme Corp')
    expect(cfg.enableDemo).toBe(false)
  })

  it('falls back to VITE_* env when window config is absent', () => {
    vi.stubEnv('VITE_API_BASE_URL', 'http://from-env:9999')
    vi.stubEnv('VITE_TENANT_MODE', 'subdomain')
    vi.stubEnv('VITE_TENANT_SLUG', 'env-tenant')
    vi.stubEnv('VITE_APP_NAME', 'EnvName')
    vi.stubEnv('VITE_ENABLE_DEMO', 'false')

    const cfg = appConfig()
    expect(cfg.apiBaseUrl).toBe('http://from-env:9999')
    expect(cfg.tenantMode).toBe('subdomain')
    expect(cfg.tenantSlug).toBe('env-tenant')
    expect(cfg.appName).toBe('EnvName')
    expect(cfg.enableDemo).toBe(false)
  })

  it('falls back per key: a partial window config keeps env fallbacks for the rest', () => {
    window.__APP_CONFIG__ = { apiBaseUrl: 'http://from-window:8080' }
    vi.stubEnv('VITE_APP_NAME', 'EnvName')

    const cfg = appConfig()
    expect(cfg.apiBaseUrl).toBe('http://from-window:8080')
    expect(cfg.appName).toBe('EnvName')
  })

  it('provides sane defaults when neither source is set', () => {
    const cfg = appConfig()
    expect(cfg.apiBaseUrl).toBe('http://localhost:8080')
    expect(cfg.tenantMode).toBe('header')
    expect(cfg.tenantSlug).toBe('')
    expect(cfg.appName).toBe('Entitlements')
    expect(cfg.enableDemo).toBe(true) // demo defaults on in dev
  })
})
