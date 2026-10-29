/**
 * OIDC setup for the browser.
 *
 * The SPA is a public client: it performs authorization code + PKCE against
 * Authelia itself and sends the resulting access token to whichever backend is
 * configured. Both backends are stateless resource servers that verify the
 * token against the issuer's JWKS, so no login flow is duplicated there.
 */
import { InMemoryWebStorage, UserManager, WebStorageStateStore } from 'oidc-client-ts'
import { loadConfig } from '@/lib/config'

let manager: Promise<UserManager> | null = null

/**
 * Where the session is kept.
 *
 * sessionStorage keeps the token out of a shared machine's disk; the token is
 * re-fetched after a reload via a silent redirect if needed.
 *
 * A browser is allowed to refuse storage, though - private windows, blocked
 * origins and enterprise policies throw, on the property itself or on the
 * first write - so the store is probed once and falls back to memory. That
 * costs the session on a reload; refusing to fall back would cost the page,
 * because the manager is built before React renders and every build would fail.
 * lib/storage.ts probes the same way for preferences.
 */
function sessionStore(): Storage {
  try {
    const store = window.sessionStorage
    const probe = '__oidc_probe__'
    store.setItem(probe, '1')
    store.removeItem(probe)
    return store
  } catch {
    return new InMemoryWebStorage()
  }
}

export function getUserManager(): Promise<UserManager> {
  if (manager) return manager

  manager = loadConfig()
    .then(
      (config) =>
        new UserManager({
          authority: config.oidc.issuer,
          client_id: config.oidc.clientId,
          redirect_uri: `${window.location.origin}${config.oidc.redirectPath}`,
          post_logout_redirect_uri: window.location.origin,
          response_type: 'code',
          scope: config.oidc.scope,
          // Authelia puts only the standard claims in its tokens, so `sub` would
          // be all the UI could show. Pulling userinfo merges name, email and
          // groups into the profile, which needs no claims policy at the
          // provider. It costs one request per sign-in.
          loadUserInfo: true,
          userStore: new WebStorageStateStore({ store: sessionStore() }),
          automaticSilentRenew: false,
        })
    )
    // A failed build is not kept: this promise is handed to every later caller,
    // so caching the rejection would make each of them fail too, including the
    // API client's token lookup. The next caller builds again.
    .catch((error: unknown) => {
      manager = null
      throw error
    })

  return manager
}
