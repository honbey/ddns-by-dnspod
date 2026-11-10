/**
 * What to show a person for a failed call.
 *
 * Beside the kernel rather than inside it because it needs the dictionary:
 * `lib/http.ts` hands back an `ApiError` and this turns it into a sentence.
 */
import { i18n } from '@/i18n'
import { ApiError } from '@/lib/http'

/**
 * `fallback` names the action that just failed, which a generic string cannot.
 */
export function describeError(
  error: unknown,
  fallback = i18n.t('common.somethingWentWrong')
): string {
  if (!(error instanceof ApiError)) {
    return error instanceof Error && error.message ? error.message : fallback
  }

  // A code names the failures a UI can put in the reader's language. Other
  // conflicts keep the server's words: they say which resource collided, and
  // a generic translation would lose that.
  const text =
    error.code === 'duplicate_task' ? i18n.t('common.duplicateTask') : error.message || fallback
  return error.requestId
    ? i18n.t('common.withRequestId', { message: text, requestId: error.requestId })
    : text
}
