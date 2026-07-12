/**
 * Contract test for the Docker runtime-config injection (F-002).
 *
 * The container renders `app-config.js` from `app-config.js.template` at start
 * via `envsubst`, using defaults declared in `docker-entrypoint.sh`. This test
 * reads those two real files, replays the substitution, and asserts that:
 *   - every documented variable has a default in the entrypoint,
 *   - the template references exactly those variables,
 *   - rendering with no env yields valid JS defining `window.__APP_CONFIG__`
 *     with each documented value,
 *   - overriding an env var flows through to the output (the whole point of the
 *     "one generic image, config injected at start" design).
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Vitest runs with the admin/ project root as cwd (same as `npm test` in CI),
// so the Docker files sit directly under it.
const templatePath = resolve(process.cwd(), 'app-config.js.template')
const entrypointPath = resolve(process.cwd(), 'docker-entrypoint.sh')

const template = readFileSync(templatePath, 'utf8')
const entrypoint = readFileSync(entrypointPath, 'utf8')

/** Every runtime variable documented in docs/FRONTEND.md §3 and the F-002 card. */
const DOCUMENTED_VARS = [
  'API_BASE_URL',
  'TENANT_MODE',
  'TENANT_SLUG',
  'APP_NAME',
  'ENABLE_DEMO',
] as const

/** Parse `: "${VAR:=default}"` default declarations out of the entrypoint. */
function parseDefaults(script: string): Record<string, string> {
  const defaults: Record<string, string> = {}
  const re = /:\s*"\$\{([A-Z_]+):=([^}]*)\}"/g
  for (const m of script.matchAll(re)) {
    defaults[m[1]] = m[2]
  }
  return defaults
}

/** Replay `envsubst` for our known variables. */
function render(env: Record<string, string> = {}): string {
  const defaults = parseDefaults(entrypoint)
  let out = template
  for (const name of DOCUMENTED_VARS) {
    const value = env[name] ?? defaults[name] ?? ''
    out = out.replaceAll(`\${${name}}`, value)
  }
  return out
}

/** Evaluate the rendered app-config.js against a fake window. */
function evalConfig(rendered: string): Record<string, unknown> {
  const win: { __APP_CONFIG__?: Record<string, unknown> } = {}
  new Function('window', rendered)(win)
  if (!win.__APP_CONFIG__) throw new Error('rendered file did not set window.__APP_CONFIG__')
  return win.__APP_CONFIG__
}

describe('docker runtime app-config template', () => {
  it('declares a default for every documented variable', () => {
    const defaults = parseDefaults(entrypoint)
    for (const name of DOCUMENTED_VARS) {
      expect(defaults, `missing default for ${name}`).toHaveProperty(name)
    }
  })

  it('the template references exactly the documented variables', () => {
    const referenced = [...template.matchAll(/\$\{([A-Z_]+)\}/g)].map((m) => m[1])
    expect(new Set(referenced)).toEqual(new Set(DOCUMENTED_VARS))
  })

  it('renders every documented variable with its default (no placeholders left)', () => {
    const rendered = render()
    expect(rendered).not.toMatch(/\$\{/) // nothing left un-substituted

    const cfg = evalConfig(rendered)
    expect(cfg).toEqual({
      apiBaseUrl: 'http://localhost:8080',
      tenantMode: 'header',
      tenantSlug: '',
      appName: 'Entitlements',
      enableDemo: false,
    })
    // enableDemo must be a real boolean, not the string "false".
    expect(typeof cfg.enableDemo).toBe('boolean')
  })

  it('flows env overrides through to the rendered config', () => {
    const cfg = evalConfig(
      render({
        API_BASE_URL: 'https://api.other.example',
        TENANT_MODE: 'subdomain',
        TENANT_SLUG: 'acme',
        APP_NAME: 'Acme Admin',
        ENABLE_DEMO: 'true',
      }),
    )
    expect(cfg).toEqual({
      apiBaseUrl: 'https://api.other.example',
      tenantMode: 'subdomain',
      tenantSlug: 'acme',
      appName: 'Acme Admin',
      enableDemo: true,
    })
  })
})
