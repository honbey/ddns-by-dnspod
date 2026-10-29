/** What this deployment itself looks like from the outside. */
import { request } from '@/lib/http'
import type { PublicIP } from '@/lib/types'

/** What the host is reachable as right now. */
export function currentIP(family: 'v4' | 'v6'): Promise<PublicIP> {
  return request<PublicIP>({ method: 'get', url: '/ip', params: { family } })
}
