import { createFileRoute } from '@tanstack/react-router'
import { Records } from '@/features/records'

/**
 * `account_id` names the account the domain belongs to.
 *
 * It has to travel in the URL because the records page cannot guess it: the
 * same name can exist under two accounts, and the API answers for the default
 * account when the parameter is missing. Absent is therefore the default
 * account, which is what a link from anywhere else means.
 */
export const Route = createFileRoute('/_authenticated/domains/$domain')({
  validateSearch: (search: Record<string, unknown>): { account_id?: number } => {
    const id = Number(search.account_id)
    return { account_id: Number.isInteger(id) && id > 0 ? id : undefined }
  },
  component: Records,
})
