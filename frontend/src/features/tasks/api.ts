/** DDNS tasks: the records this service keeps pointed at this host. */
import { request } from '@/lib/http'
import { API_PAGE_SIZE, collectPages, type CollectedList } from '@/lib/lists'
import type { DDNSTask, ImportResult, PagedDDNSTask, RunResult, TaskBatchResult } from '@/lib/types'

export interface TaskInput {
  account_id: number
  domain: string
  sub_domain: string
  record_type?: string
  record_line?: string
  ttl?: number
  enabled?: boolean
  cron_expr?: string
  notify_on_change?: boolean
}

export interface TaskQuery {
  page?: number
  page_size?: number
  domain?: string
  keyword?: string
  /** Last outcome, e.g. FAIL; lets a caller count without reading every task. */
  status?: string
}

export function listTasks(query: TaskQuery = {}): Promise<PagedDDNSTask> {
  return request<PagedDDNSTask>({
    method: 'get',
    url: '/ddns/tasks',
    params: query,
  })
}

/**
 * Every page of the task list.
 *
 * The table filters, sorts and paginates client side, so a page that stopped at
 * two hundred rows would decide, invisibly, which tasks the page can act on -
 * "Run all" included.
 */
export function listAllTasks(query: TaskQuery = {}): Promise<CollectedList<DDNSTask>> {
  return collectPages((page) => listTasks({ ...query, page, page_size: API_PAGE_SIZE }))
}

export function getTask(id: number): Promise<DDNSTask> {
  return request<DDNSTask>({ method: 'get', url: `/ddns/tasks/${id}` })
}

/** One task per host; the server skips the ones that already exist. */
export interface BatchTaskInput {
  account_id: number
  domain: string
  sub_domains: string[]
  record_type?: string
  record_line?: string
  ttl?: number
  enabled?: boolean
  cron_expr?: string
  notify_on_change?: boolean
}

export function batchCreateTasks(input: BatchTaskInput): Promise<TaskBatchResult> {
  return request<TaskBatchResult>({ method: 'post', url: '/ddns/tasks/batch', data: input })
}

export function updateTask(id: number, input: TaskInput): Promise<DDNSTask> {
  return request<DDNSTask>({ method: 'put', url: `/ddns/tasks/${id}`, data: input })
}

export function deleteTask(id: number): Promise<null> {
  return request<null>({ method: 'delete', url: `/ddns/tasks/${id}` })
}

/** Runs one task now, whatever the schedule says. */
export function runTask(id: number): Promise<RunResult> {
  return request<RunResult>({ method: 'post', url: `/ddns/tasks/${id}/run` })
}

export interface ImportInput {
  group_id: number
  account_id: number
  record_type?: string
  cron_expr?: string
  enabled?: boolean
}

/** Turns every address record of one DNSPod group into a task. */
export function importTasksFromGroup(domain: string, input: ImportInput): Promise<ImportResult> {
  return request<ImportResult>({
    method: 'post',
    url: `/ddns/tasks/import-group/${encodeURIComponent(domain)}`,
    data: input,
  })
}

/** Re-reads the tasks and rebuilds the in-process cron schedule. */
export function reloadScheduler(): Promise<null> {
  return request<null>({ method: 'post', url: '/ddns/reload' })
}
