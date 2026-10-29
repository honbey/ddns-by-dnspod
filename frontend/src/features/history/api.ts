/**
 * The two change histories: one per task, one per host.
 *
 * They answer different questions, which is why they are separate endpoints -
 * what this service wrote to DNS, and what the address was when it wrote it.
 */
import { request, type Query } from '@/lib/http'
import type { IPChangePoint, PagedHistoryEntry } from '@/lib/types'

export interface HistoryQuery {
  page?: number
  page_size?: number
  task_id?: number
  domain?: string
  status?: string
}

export function listHistory(query: HistoryQuery = {}): Promise<PagedHistoryEntry> {
  return request<PagedHistoryEntry>({ method: 'get', url: '/history', params: query })
}

/**
 * Recent public address changes, oldest first so the chart reads left to right.
 * Only real changes appear: the first address of a family has nothing to measure
 * its lifetime against yet.
 */
export function listIPChanges(limit = 30, family?: string): Promise<IPChangePoint[]> {
  const params: Query = { limit, ...(family ? { family } : {}) }
  return request<IPChangePoint[]>({
    method: 'get',
    url: '/stats/ip-changes',
    params,
  })
}
