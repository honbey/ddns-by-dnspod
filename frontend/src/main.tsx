import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { toast } from 'sonner'
import { discardSession, initAuth, refreshProviderLogoutAvailability } from '@/lib/auth'
import { setApiBaseUrl } from '@/lib/http'
import { createLatch } from '@/lib/latch'
import { createQueryClient } from '@/lib/query-client'
import { safeReturnPath } from '@/lib/redirect'
import { onRemoteSignOut } from '@/lib/session-events'
import { authRequired, loadConfig } from '@/lib/config'
import { i18n, i18nReady } from '@/i18n'
import { setUnauthorizedHandler } from '@/lib/handle-server-error'
import { DirectionProvider } from './context/direction-provider'
import { FontProvider } from './context/font-provider'
import { ThemeProvider } from './context/theme-provider'
// Generated Routes
import { routeTree } from './routeTree.gen'
// Styles
import './styles/index.css'

const queryClient = createQueryClient(sessionExpired)

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
})

// Register the router instance for type safety
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// While one expiry is in flight, the others stay quiet: "Run All" fires a
// mutation per task, and without this each one would toast and navigate on its
// own. The latch opens when the navigation settles - or never on the sign-in
// page, where a successful login reloads this module and the latch with it.
const expiryLatch = createLatch()

/**
 * The one way out of a dead session, shared by queries and mutations alike.
 *
 * A mutation never reaches the query cache, so it needs the same door a failed
 * card gets: reset the session and offer to come back to where the user was.
 */
function sessionExpired(): void {
  if (!expiryLatch.tryAcquire()) return
  if (!authRequired()) {
    // Nothing was ever signed, so there is no session to expire and nowhere to
    // send the user. The failure was diverted here by the mutation handler,
    // which has already decided not to toast it, so it has to be reported here
    // - and a request that was turned away is not an expiry.
    expiryLatch.release()
    toast.error(i18n.t('common.serverRejected'))
    return
  }

  toast.error(i18n.t('common.sessionExpired'))
  // The stored session goes as well, not just the one in memory: otherwise a
  // reload reads the same rejected token back out of sessionStorage, the guard
  // lets the page through on it, and every request fails again.
  void discardSession()

  // Already on the sign-in page: there is nowhere to navigate, and a successful
  // login is a full-page redirect that reloads this module, so the latch is
  // released with the page rather than here.
  if (router.history.location.pathname === '/sign-in') return

  const redirect = safeReturnPath(router.history.location.href)
  void router.navigate({ to: '/sign-in', search: { redirect } }).finally(() => {
    // A navigation that failed left the user on the old page, where a later
    // 401 still has to be reported. Once on /sign-in, only the login reload
    // opens the latch.
    if (router.history.location.pathname !== '/sign-in') expiryLatch.release()
  })
}

setUnauthorizedHandler(sessionExpired)

// Another tab signed out: this one drops the session and the cached data with
// it, and deliberately does not broadcast the news back - discardSession is the
// quiet teardown, signOut is the loud one.
onRemoteSignOut(() => {
  queryClient.clear()
  void discardSession().then(() => {
    void router.navigate({ to: '/sign-in', search: { redirect: '/' } })
  })
})

/**
 * Resolves the runtime configuration and the session, then renders.
 *
 * Both finish before the router mounts, so a route guard can answer "is there a
 * session?" synchronously and the authorization code in the URL never reaches
 * the router.
 */
async function bootstrap() {
  // Ahead of the config and the session: the dictionaries are local, and
  // rendering one frame with untranslated keys before they land is a flash
  // nobody needs to see.
  await i18nReady

  const config = await loadConfig()
  setApiBaseUrl(config.apiBaseUrl.trim() || '/api')
  document.title = config.appName
  await initAuth()

  // Asynchronous on purpose: the answer only drives one menu entry, so it must
  // not delay the first render.
  void refreshProviderLogoutAvailability()

  const rootElement = document.getElementById('root')!
  if (rootElement.innerHTML) return

  ReactDOM.createRoot(rootElement).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <FontProvider>
            <DirectionProvider>
              <RouterProvider router={router} />
            </DirectionProvider>
          </FontProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </StrictMode>
  )
}

// bootstrap runs before React does, so a rejection here would be a blank page
// with one console entry and no way to reach even the sign-in route. Whatever
// went wrong is shown in place of the app instead.
void bootstrap().catch((error: unknown) => {
  const root = document.getElementById('root')
  if (!root) return
  const reason = error instanceof Error ? error.message : String(error)
  root.textContent = `The app could not start: ${reason}. Reload the page to try again.`
})
