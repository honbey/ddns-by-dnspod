/** The domains an account holds; records live in ./records. */
import { request } from '@/lib/http'
import { accountParam } from '@/features/accounts/api'
import type { Domain, DomainGroup } from '@/lib/types'

export function listDomains(accountId?: number): Promise<Domain[]> {
  return request<Domain[]>({
    method: 'get',
    url: '/domains',
    params: accountParam(accountId),
  })
}

/** The domain groups of an account, which span domains rather than records. */
export function listDomainGroups(accountId?: number): Promise<DomainGroup[]> {
  return request<DomainGroup[]>({
    method: 'get',
    url: '/domain-groups',
    params: accountParam(accountId),
  })
}
