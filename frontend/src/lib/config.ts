/**
 * Runtime configuration.
 *
 * The backend address and OIDC parameters live in public/config.json rather
 * than being baked in at build time, so one build can be pointed at either
 * backend (Go self-hosted or EdgeOne) without rebuilding.
 */

interface OidcConfig {
  issuer: string
  clientId: string
  scope: string
  redirectPath: string
  /**
   * Name of the identity provider as it should appear in the UI. Discovery
   * documents carry no provider name, so this is configuration; an empty value
   * falls back to `OIDC`.
   */
  providerName: string
  /**
   * Sign-out URL for providers that implement no OIDC logout and therefore
   * expose no end_session_endpoint, e.g.
   * `https://auth.example.com/logout?rd={redirectUrl}`. `{redirectUrl}` is
   * replaced with the URL-encoded return target. Empty hides the
   * "sign out everywhere" entry.
   */
  logoutUrlTemplate: string
}

export interface AppConfig {
  appName: string
  apiBaseUrl: string
  /**
   * Whether the API demands an access token.
   *
   * False matches the server's AUTH_DISABLED=true, the local development
   * setting: the guard then lets the pages through and the sign-in route sends
   * a stray visitor home instead of to a provider nobody deployed.
   */
  authRequired: boolean
  /**
   * Whether this deployment issues certificates.
   *
   * False matches the server's acme.enabled=false: nothing is issued or
   * renewed there, so no page offers a button the API would answer 503 for.
   */
  acmeEnabled: boolean
  /**
   * Schedule a new certificate gets when it names none.
   *
   * The server seeds this from acme.check_hour; the form offers it as the
   * starting value so the deployment's hour is not repeated and then overridden
   * by a copy of it in the bundle.
   */
  acmeDefaultRenewCron: string
  oidc: OidcConfig
}

const DEFAULTS: AppConfig = {
  appName: 'DDNS by DNSPod',
  apiBaseUrl: '/api',
  authRequired: true,
  // A deployment that says nothing about certificates gets the buttons, and the
  // server has the last word: it refuses an issuance it has been told not to
  // make whatever this document claims.
  acmeEnabled: true,
  acmeDefaultRenewCron: '0 3 1 * *',
  oidc: {
    issuer: 'https://auth.example.com',
    clientId: 'ddns-by-dnspod',
    scope: 'openid profile email groups',
    redirectPath: '/auth/callback',
    providerName: '',
    logoutUrlTemplate: '',
  },
}

let cached: AppConfig | null = null
let pending: Promise<AppConfig> | null = null

/**
 * `config.local.json` is tried first so a checkout can be pointed somewhere
 * without touching a tracked file; it is gitignored and absent from builds, in
 * which case `config.json` is used.
 */
const CONFIG_FILES = ['config.local.json', 'config.json']

/** How long the runtime configuration may take. */
const CONFIG_TIMEOUT_MS = 10_000

async function fetchConfigFile(file: string): Promise<Partial<AppConfig> | null> {
  try {
    const response = await fetch(`${import.meta.env.BASE_URL}${file}`, {
      cache: 'no-store',
      // Bounded: bootstrap awaits this, and a request that never answers would
      // leave the root element empty with nothing to say why.
      signal: AbortSignal.timeout(CONFIG_TIMEOUT_MS),
    })
    if (!response.ok) return null

    const raw = (await response.json()) as unknown
    // Both this dev server and every SPA host answer unknown paths with
    // index.html, so a 200 is not proof the file exists.
    return typeof raw === 'object' && raw !== null ? (raw as Partial<AppConfig>) : null
  } catch {
    return null
  }
}

/** Loads and caches the runtime configuration. */
export function loadConfig(): Promise<AppConfig> {
  if (cached) return Promise.resolve(cached)
  if (pending) return pending

  pending = (async () => {
    for (const file of CONFIG_FILES) {
      const raw = await fetchConfigFile(file)
      if (raw) return raw
    }
    return null
  })()
    .then((raw) => {
      // A missing config file still yields a usable app so the sign-in page
      // can render instead of a blank screen.
      cached = {
        ...DEFAULTS,
        ...raw,
        oidc: { ...DEFAULTS.oidc, ...(raw?.oidc ?? {}) },
      }
      return cached
    })
    .catch(() => {
      cached = DEFAULTS
      return cached
    })

  return pending
}

/**
 * Synchronous access to the already-loaded configuration.
 *
 * `bootstrap()` resolves the config before the router renders, so components
 * can read it without suspending.
 */
export function getConfig(): AppConfig {
  return cached ?? DEFAULTS
}

/**
 * Whether a bearer token has to be presented.
 *
 * Read by the route guard, which has to answer synchronously: everything is
 * already resolved by the time a route renders.
 */
export function authRequired(): boolean {
  return getConfig().authRequired
}

/**
 * Whether certificates can be issued here.
 *
 * Read by the certificates page, which has to answer synchronously - and a
 * button that only comes back 503 is worse than one that says why it is off.
 */
export function acmeEnabled(): boolean {
  return getConfig().acmeEnabled
}

/** The schedule the server gives a certificate that names none. */
export function defaultRenewCron(): string {
  return getConfig().acmeDefaultRenewCron
}

/**
 * The hour that schedule runs at.
 *
 * Read out of the expression rather than configured twice: the shortcuts a
 * certificate form offers are built from it, so they name the same hour the
 * deployment does. Falls back to 03:00 if the document carries something the
 * parser cannot read.
 */
export function defaultRenewHour(): number {
  const hour = Number(defaultRenewCron().trim().split(/\s+/)[1])
  return Number.isInteger(hour) && hour >= 0 && hour <= 23 ? hour : 3
}

/**
 * Provider name for UI copy.
 *
 * Falls back to the generic `OIDC` rather than the issuer host: a host is an
 * address, not a name, and it would surface a placeholder like
 * `auth.example.com` in sentences.
 */
export function providerName(): string {
  return getConfig().oidc.providerName || 'OIDC'
}
