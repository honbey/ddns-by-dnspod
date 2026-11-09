/**
 * Session lifecycle.
 *
 * Owns the OIDC user and hands access tokens to the API client. The callback
 * redirect is consumed here so no route has to know about the flow.
 */
import type { User, UserManager } from 'oidc-client-ts'
import { useAuthStore } from '@/stores/auth-store'
import { getConfig, loadConfig } from '@/lib/config'
import { safeReturnPath } from '@/lib/redirect'
import { broadcastSignOut } from '@/lib/session-events'
import { getUserManager } from '@/lib/oidc'

/**
 * Which session a sign-out ends.
 *
 * `app` is everything OIDC offers for a single application: clear this
 * browser's state and revoke the grant this app holds. The logout
 * specifications define no per-RP logout at the provider, so there is nothing
 * else to call.
 *
 * `provider` ends the user's session at the provider itself. Providers that
 * implement RP-Initiated Logout get the standard end_session_endpoint - the
 * spec is explicit that the OP logs the end-user out, which affects every app
 * sharing that session. Providers implementing none of the logout
 * specifications (Authelia) get a configured URL instead.
 */
export type LogoutScope = 'app' | 'provider'

/** How long one revocation attempt may take. */
const REVOKE_TIMEOUT_MS = 5_000
/** How long provider metadata may take; it must not hold a sign-out open. */
const METADATA_TIMEOUT_MS = 5_000
/** How long the callback exchange may take before the page gives up. */
const CALLBACK_TIMEOUT_MS = 10_000

function withDeadline<T>(work: Promise<T>, ms: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('authentication request timed out')), ms)
    work.then(
      (value) => {
        clearTimeout(timer)
        resolve(value)
      },
      (error: unknown) => {
        clearTimeout(timer)
        reject(error)
      }
    )
  })
}

/**
 * Best-effort grant revocation (RFC 7009).
 *
 * This is the only standard way to end *one* application's access, so it is
 * what "sign out" does beyond clearing local state. Revoking is also what makes
 * a user who declines the provider's own logout confirmation page still end up
 * with nothing usable in this app.
 *
 * Failures are ignored on purpose: logout must never depend on the network.
 */
async function revokeGrant(manager: UserManager, user: User | null): Promise<void> {
  if (!user) return

  const endpoint = await Promise.race([
    manager.metadataService.getRevocationEndpoint(true).catch(() => undefined),
    new Promise<undefined>((resolve) => setTimeout(() => resolve(undefined), REVOKE_TIMEOUT_MS)),
  ])
  if (!endpoint) return

  const { clientId } = getConfig().oidc
  const tokens: Array<[string, 'refresh_token' | 'access_token']> = []
  if (user.refresh_token) tokens.push([user.refresh_token, 'refresh_token'])
  if (user.access_token) tokens.push([user.access_token, 'access_token'])

  for (const [token, hint] of tokens) {
    await fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      signal: AbortSignal.timeout(REVOKE_TIMEOUT_MS),
      body: new URLSearchParams({
        token,
        token_type_hint: hint,
        client_id: clientId,
      }),
    }).catch(() => undefined)
  }
}

/**
 * Resolves the session before the router mounts.
 *
 * When the current URL is the OIDC redirect target the authorization code is
 * exchanged first and the query string is dropped, so a refresh never replays
 * the code.
 */
export async function initAuth(): Promise<void> {
  const { oidc } = await loadConfig()
  const manager = await getUserManager()
  const { setUser } = useAuthStore.getState().auth

  if (window.location.pathname === oidc.redirectPath) {
    try {
      const user = await withDeadline(manager.signinCallback(), CALLBACK_TIMEOUT_MS)
      setUser(user ?? null)
      // The pre-login path travels in `state`, through the provider and back as
      // a query parameter - so a crafted link can reach it. safeReturnPath is
      // the one place that decides what a path on this origin means.
      window.history.replaceState({}, '', safeReturnPath(user?.state))
    } catch {
      setUser(null)
      window.history.replaceState({}, '', '/')
    }
    return
  }

  setUser(await manager.getUser())
}

/** The renew in flight, shared so a page firing several requests renews once. */
let renewing: Promise<string | null> | null = null
/** Bumped on sign-out, so a renew that resolves afterwards cannot win the session back. */
let epoch = 0

/** How long a silent renew may take before every waiting request is let go. */
const RENEW_TIMEOUT_MS = 15000

/** Resolves the current access token, refreshing it when it has expired. */
export function getAccessToken(): Promise<string | null> {
  // One renew at a time: with refresh token rotation the spare exchanges spend
  // a token that is already gone, and the one that fails resets a session that
  // was perfectly fine.
  renewing ??= renewAccessToken().finally(() => {
    renewing = null
  })
  return renewing
}

async function renewAccessToken(): Promise<string | null> {
  const manager = await getUserManager()
  const current = await manager.getUser()
  if (!current) return null

  if (!current.expired) return current.access_token

  const mine = epoch
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    // signinSilent has no deadline of its own, so a provider that never answers
    // would hold every request's interceptor open forever without one.
    const renewed = await Promise.race([
      manager.signinSilent(),
      new Promise<never>((_resolve, reject) => {
        timer = setTimeout(() => reject(new Error('silent renew timed out')), RENEW_TIMEOUT_MS)
      }),
    ])
    if (!renewed) throw new Error('silent renew returned no user')
    // A sign-out that happened while this was in flight must not be undone by
    // the grant it raced.
    if (mine !== epoch) return null
    useAuthStore.getState().auth.setUser(renewed)
    return renewed.access_token
  } catch {
    if (mine === epoch) useAuthStore.getState().auth.reset()
    return null
  } finally {
    if (timer !== undefined) clearTimeout(timer)
  }
}

/** Starts the authorization code + PKCE redirect. */
export async function signIn(returnTo?: string): Promise<void> {
  const manager = await getUserManager()
  await manager.signinRedirect({
    state: returnTo || '/',
    // Ask for the audience on both legs of the code flow: Authelia's explicit
    // mode grants it on the access (token) request, and the authorize request
    // is what caps it.
    extraQueryParams: { audience: getConfig().oidc.clientId },
    extraTokenParams: { audience: getConfig().oidc.clientId },
  })
}

/**
 * Resolves the provider-session logout target, or null when the provider has
 * no way to end its own session.
 *
 * Standards first: discovery advertising an `end_session_endpoint` means
 * RP-Initiated Logout, which ends the session at the provider. The redirect
 * back is only honoured with an `id_token_hint` (and a target registered at the
 * provider), so both are sent whenever they exist.
 */
async function providerLogoutUrl(returnTo: string): Promise<string | null> {
  const { oidc } = getConfig()
  const manager = await getUserManager()

  const endSession = await withDeadline(
    manager.metadataService.getEndSessionEndpoint(),
    METADATA_TIMEOUT_MS
  ).catch(() => undefined)

  if (endSession) {
    const url = new URL(endSession)
    url.searchParams.set('client_id', oidc.clientId)
    url.searchParams.set('post_logout_redirect_uri', returnTo)

    const user = await manager.getUser()
    if (user?.id_token) url.searchParams.set('id_token_hint', user.id_token)
    return url.toString()
  }

  if (!oidc.logoutUrlTemplate) return null
  return oidc.logoutUrlTemplate.replace(/\{redirectUrl\}/g, encodeURIComponent(returnTo))
}

/**
 * Publishes whether this provider can end its own session, so the UI knows
 * whether to offer "sign out everywhere" at all.
 *
 * A configured template answers this without a network call; otherwise the
 * discovery document decides. A failure means hiding the entry rather than
 * offering something that cannot work.
 */
export async function refreshProviderLogoutAvailability(): Promise<void> {
  const { setProviderLogout } = useAuthStore.getState().auth

  try {
    const { oidc } = getConfig()
    if (oidc.logoutUrlTemplate) {
      setProviderLogout(true)
      return
    }

    const manager = await getUserManager()
    const endSession = await withDeadline(
      manager.metadataService.getEndSessionEndpoint(),
      METADATA_TIMEOUT_MS
    )
    setProviderLogout(Boolean(endSession))
  } catch {
    setProviderLogout(false)
  }
}

/**
 * Signs out.
 *
 * The provider-session target is resolved before local state is dropped,
 * because the end_session redirect needs the id_token. Revoking happens before
 * the navigation so an interrupted trip cannot leave a usable grant behind.
 */
export async function signOut(scope: LogoutScope = 'app'): Promise<void> {
  let manager: Awaited<ReturnType<typeof getUserManager>> | null = null
  let target: string | null = null
  try {
    manager = await getUserManager()
    const user = await manager.getUser()

    if (scope === 'provider') {
      target = await providerLogoutUrl(`${window.location.origin}/`)
    }

    // Bounded as a whole as well: the local half is what the user asked for.
    await Promise.race([
      revokeGrant(manager, user),
      new Promise((resolve) => setTimeout(resolve, REVOKE_TIMEOUT_MS)),
    ])
  } catch {
    // Provider trouble must not keep the local session alive.
  } finally {
    if (manager) {
      await manager.removeUser().catch(() => undefined)
    }
    // Before reset, so a silent renew still in flight loses the epoch check.
    epoch += 1
    useAuthStore.getState().auth.reset()
    broadcastSignOut()
  }

  window.location.assign(target ?? '/')
}

/**
 * Forgets the stored session, for the paths that end it without a sign-out.
 *
 * Clearing the in-memory user alone is not enough: the token lives in
 * sessionStorage, so the next page load reads it back, walks past the route
 * guard and fails every request again - which is what a session the server has
 * already rejected looks like from the browser's side.
 */
export async function discardSession(): Promise<void> {
  // Before the reset, as in signOut: a silent renew that is still in flight has
  // to lose the epoch check, or it puts the session back once it is gone.
  epoch += 1
  useAuthStore.getState().auth.reset()

  try {
    const manager = await getUserManager()
    await manager.removeUser()
  } catch {
    // No manager means nothing was ever stored; the reset above stands.
  }
}

/**
 * The claims this app reads off the OIDC profile.
 *
 * Declared here rather than on `User` because they are the app's needs, not the
 * library's: `loadUserInfo` merges the userinfo response into `profile`, which is
 * where `groups` comes from (`oidc.ts`), and `sub` always does. The identity is
 * read from here instead of from a `/me` endpoint so a storage backend with no
 * server of its own - an `s3` bucket - can still tell who is using it.
 */
export interface ProfileClaims {
  sub?: string
  name?: string
  preferred_username?: string
  email?: string
  groups?: string[]
}

/** The profile of the current session, or an empty one when signed out. */
export function profileOf(user: User | null): ProfileClaims {
  return (user?.profile ?? {}) as ProfileClaims
}

/**
 * Display name for the current session, preferring the profile claim.
 *
 * `sub` is last: Authelia's default tokens carry only the standard claims, so
 * without a claims policy this is an opaque UUID that reads badly in the UI.
 */
export function displayName(user: User | null): string {
  if (!user) return 'Signed out'
  const profile = profileOf(user)
  return profile.name ?? profile.preferred_username ?? profile.email ?? profile.sub ?? 'Signed in'
}
