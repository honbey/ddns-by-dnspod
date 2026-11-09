/**
 * The two change histories: one per task, one per host.
 *
 * They answer different questions, which is why they are separate endpoints -
 * what this service wrote to DNS, and what the address was when it wrote it.
 */
import { request, type Query } from '@/lib/http'
import type {
  IPChangePoint,
  IPHistoryImportResult,
  PagedHistoryEntry,
  PagedIPChange,
} from '@/lib/types'

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
/** The two timelines; the contract's family enum. */
export type IPFamily = 'v4' | 'v6'

export function listIPChanges(limit = 30, family?: IPFamily): Promise<IPChangePoint[]> {
  const params: Query = { limit, ...(family ? { family } : {}) }
  return request<IPChangePoint[]>({
    method: 'get',
    url: '/stats/ip-changes',
    params,
  })
}

export interface IPHistoryQuery {
  page?: number
  page_size?: number
  family?: IPFamily
}

/**
 * Every address change, newest first, baseline rows included - the table can
 * show the first address of a family even though the chart has no bar for it.
 */
export function listIPHistory(query: IPHistoryQuery = {}): Promise<PagedIPChange> {
  return request<PagedIPChange>({ method: 'get', url: '/history/ip-changes', params: query })
}

/**
 * Merges a CSV of `time,ip[,family]` into the timeline. The family is inferred
 * from the address when the column is missing; rows that cannot be read are
 * reported in the result rather than failing the whole file.
 */
export function importIPHistory(file: File): Promise<IPHistoryImportResult> {
  const form = new FormData()
  form.append('file', file)
  // No Content-Type here: the browser has to add the multipart boundary.
  return request<IPHistoryImportResult>({
    method: 'post',
    url: '/history/ip-changes/import',
    data: form,
  })
}
