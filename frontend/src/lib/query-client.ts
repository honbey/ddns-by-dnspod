import { QueryCache, QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { authRequired } from '@/lib/config'
import { describeError } from '@/lib/describe-error'
import { handleServerError } from '@/lib/handle-server-error'
import { ApiError } from '@/lib/http'
import { readStored, removeStored, writeStored } from '@/lib/storage'

/** `no_account` in openapi.yml, `api.CodeNoAccount` on the server. */
const MISSING_ACCOUNT = 'no_account'

const MISSING_ACCOUNT_NOTICE = 'missing-account-notice'

function isMissingAccount(error: unknown): boolean {
  return error instanceof ApiError && error.code === MISSING_ACCOUNT
}

/** Re-armed when an account is saved. */
export function clearMissingAccountNotice(): void {
  removeStored(MISSING_ACCOUNT_NOTICE)
}

export function createQueryClient(onUnauthorized: () => void): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => {
          const refused = error instanceof ApiError && [401, 403].includes(error.status)
          return failureCount < 2 && !refused
        },
        refetchOnWindowFocus: import.meta.env.PROD,
        staleTime: 10 * 1000,
      },
      // TanStack passes mutation variables as a second argument, which would
      // be read as handleServerError's fallback wording.
      mutations: { onError: (error) => handleServerError(error) },
    },
    queryCache: new QueryCache({
      onError: (error) => {
        if (error instanceof ApiError && error.status === 401 && authRequired()) {
          onUnauthorized()
          return
        }

        // Show the missing-account notice once per browser; later refetches
        // leave it to the page's error state.
        if (isMissingAccount(error)) {
          if (readStored(MISSING_ACCOUNT_NOTICE)) return
          writeStored(MISSING_ACCOUNT_NOTICE, '1')
        }

        toast.error(describeError(error))
      },
    }),
  })
}
