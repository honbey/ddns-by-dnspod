/**
 * The query client, built rather than declared.
 *
 * Taking the 401 handler as an argument is what keeps this module out of the
 * router's business: the client reports, the caller decides where the user
 * goes. The values themselves are the house rules every query inherits, so a
 * page cannot get them subtly wrong by forgetting one.
 */
import { QueryCache, QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { authRequired } from '@/lib/config'
import { describeError } from '@/lib/describe-error'
import { handleServerError } from '@/lib/handle-server-error'
import { ApiError } from '@/lib/http'
import { readStored, removeStored, writeStored } from '@/lib/storage'

/**
 * The server's code for a request that needs credentials it has not been given
 * yet (the `no_account` enum in openapi.yml, `api.CodeNoAccount` on the Go
 * side). A code rather than the sentence, so the wording can be reworded or
 * translated without this notice silently turning back into one per refetch.
 */
const MISSING_ACCOUNT = 'no_account'

/** One browser, one notice: the condition lasts until somebody adds a key. */
const MISSING_ACCOUNT_NOTICE = 'missing-account-notice'

function isMissingAccount(error: unknown): boolean {
  return error instanceof ApiError && error.code === MISSING_ACCOUNT
}

/**
 * Forgets that the notice was shown, so the next empty account list speaks
 * again. Called when an account is saved: the condition it warns about has
 * just been fixed.
 */
export function clearMissingAccountNotice(): void {
  removeStored(MISSING_ACCOUNT_NOTICE)
}

export function createQueryClient(onUnauthorized: () => void): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => {
          // A rejected token does not become valid by asking again.
          const refused = error instanceof ApiError && [401, 403].includes(error.status)
          return failureCount < 2 && !refused
        },
        refetchOnWindowFocus: import.meta.env.PROD,
        staleTime: 10 * 1000,
      },
      // Wrapped rather than passed straight through: TanStack hands the
      // mutation variables as a second argument, which would be read as this
      // handler's fallback wording.
      mutations: { onError: (error) => handleServerError(error) },
    },
    queryCache: new QueryCache({
      onError: (error) => {
        // Nothing else can renew the token, and no page works without one, so
        // a 401 is sent to the sign-in page rather than shown as a failed card.
        // It is the one failure with a way out.
        if (error instanceof ApiError && error.status === 401 && authRequired()) {
          onUnauthorized()
          return
        }

        // Every page that reads the account list fails while none exists, and
        // every refetch on focus fails again. The first one says what to do;
        // the rest would repeat that sentence until somebody adds a key, so
        // they stay quiet - the page still shows its own error state.
        if (isMissingAccount(error)) {
          if (readStored(MISSING_ACCOUNT_NOTICE)) return
          writeStored(MISSING_ACCOUNT_NOTICE, '1')
        }

        // A refetch that already has data on screen is the app keeping itself
        // fresh; failing one is not worth a toast per query, on every focus.
        toast.error(describeError(error))
      },
    }),
  })
}
