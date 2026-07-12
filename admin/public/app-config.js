// Runtime configuration — loaded from index.html BEFORE the app bundle.
// In a deployed image this file is regenerated from environment variables at
// container start (see F-002), so the same build serves any environment.
window.__APP_CONFIG__ = {
  apiBaseUrl: 'http://localhost:8080', // Go API origin
  tenantMode: 'header', // "header" | "subdomain"
  tenantSlug: '', // fixed tenant when mode=header (else resolved from subdomain)
  appName: 'Entitlements',
  enableDemo: true, // theme demo browsable under /demo (default on in dev)
}
