/**
 * Runtime application configuration.
 *
 * Deploy-time settings are injected via `public/app-config.js`, which defines
 * `window.__APP_CONFIG__` and is loaded from index.html BEFORE the bundle —
 * the same built image can therefore point at any backend without a rebuild.
 * In dev (or when a key is missing) we fall back to `import.meta.env.VITE_*`.
 */

export type TenantMode = 'header' | 'subdomain'

export type AppConfig = {
  /** Origin of the Go API, e.g. "http://localhost:8080". No trailing slash. */
  apiBaseUrl: string
  /** How the backend resolves the tenant: explicit header or subdomain. */
  tenantMode: TenantMode
  /** Fixed tenant identifier sent as X-Tenant-ID when tenantMode is "header". */
  tenantSlug: string
  /** Product name used for branding (logo alt text, page titles, …). */
  appName: string
  /** Whether the vendored Inspinia demo is browsable under /demo. */
  enableDemo: boolean
}

declare global {
  interface Window {
    __APP_CONFIG__?: Partial<AppConfig>
  }
}

const stripTrailingSlash = (url: string) => url.replace(/\/+$/, '')

export function appConfig(): AppConfig {
  const runtime = (typeof window !== 'undefined' && window.__APP_CONFIG__) || {}
  const env = import.meta.env

  return {
    apiBaseUrl: stripTrailingSlash(runtime.apiBaseUrl ?? env.VITE_API_BASE_URL ?? 'http://localhost:8080'),
    tenantMode: (runtime.tenantMode ?? env.VITE_TENANT_MODE ?? 'header') as TenantMode,
    tenantSlug: runtime.tenantSlug ?? env.VITE_TENANT_SLUG ?? '',
    appName: runtime.appName ?? env.VITE_APP_NAME ?? 'Entitlements',
    enableDemo: runtime.enableDemo ?? env.VITE_ENABLE_DEMO !== 'false',
  }
}
