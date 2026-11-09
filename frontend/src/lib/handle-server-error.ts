import { toast } from 'sonner'
import { i18n } from '@/i18n'
import { ApiError } from '@/lib/http'
import { describeError } from '@/lib/describe-error'

/**
 * Set by the app root, the only module that owns the router.
 *
 * A redirect is not this module's business: it has no router and importing one
 * would close a cycle. The root hands over the single way out instead.
 */
export function setUnauthorizedHandler(fn: () => void): void {
  onUnauthorized = fn
}

let onUnauthorized: (() => void) | undefined

/**
 * What a failed mutation says out loud.
 *
 * Mutations have no cache-level handler, so this is where they land. The server
 * puts its own words in `error` and repeats them in its log under the request
 * id, which `describeError` appends so a screenshot is enough to find the line.
 *
 * `fallback` is the wording only the caller knows: it names the action that just
 * failed, where a generic string would not.
 *
 * A 401 is a session with no way out, so it goes to the handler the app root
 * registered instead of a toast: nothing is retryable, and one message beats
 * one per request.
 */
export function handleServerError(
  error: unknown,
  fallback = i18n.t('common.somethingWentWrong')
): void {
  if (import.meta.env.DEV) {
    // eslint-disable-next-line no-console
    console.log(error)
  }

  if (error instanceof ApiError && error.status === 401) {
    onUnauthorized?.()
    return
  }

  toast.error(describeError(error, fallback))
}
