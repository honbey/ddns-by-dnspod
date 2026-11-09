/** The Tencent Cloud accounts the deployments talk to DNSPod through. */
import { request, type Query } from '@/lib/http'
import { API_PAGE_SIZE, collectPages } from '@/lib/lists'
import type { Account, PagedAccount } from '@/lib/types'

export interface AccountInput {
  name: string
  secret_id: string
  /** Blank keeps the stored key: the API never hands one back. */
  secret_key: string
  is_default?: boolean
}

/**
 * Every account, not one page.
 *
 * The API pages its lists so no call can ask for an unbounded answer; the
 * console filters and sorts client side, so it reads the pages through.
 */
export function listAccounts(): Promise<Account[]> {
  return collectPages((page) =>
    request<PagedAccount>({
      method: 'get',
      url: '/accounts',
      params: { page, page_size: API_PAGE_SIZE },
    })
  ).then((list) => list.items)
}

export function createAccount(input: AccountInput): Promise<Account> {
  return request<Account>({ method: 'post', url: '/accounts', data: input })
}

export function updateAccount(id: number, input: AccountInput): Promise<Account> {
  return request<Account>({ method: 'put', url: `/accounts/${id}`, data: input })
}

export function deleteAccount(id: number): Promise<null> {
  return request<null>({ method: 'delete', url: `/accounts/${id}` })
}

/** Asks DNSPod whether the credentials still work. */
export function verifyAccount(id: number): Promise<unknown> {
  return request<unknown>({ method: 'post', url: `/accounts/${id}/verify` })
}

/** `account_id`, which nearly every endpoint accepts and few require. */
export function accountParam(accountId?: number): Query {
  return accountId ? { account_id: accountId } : {}
}
