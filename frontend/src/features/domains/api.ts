/** The domains an account holds; records live in ./records. */
import { request } from '@/lib/http'
import { accountParam } from '@/features/accounts/api'
import type { Domain } from '@/lib/types'

export function listDomains(accountId?: number): Promise<Domain[]> {
  return request<Domain[]>({
    method: 'get',
    url: '/domains',
    params: accountParam(accountId),
  })
}
