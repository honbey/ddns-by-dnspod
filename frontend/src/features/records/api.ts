/** DNS records of one domain. */
import { request } from '@/lib/http'
import { accountParam } from '@/features/accounts/api'
import { API_PAGE_SIZE, collectPages, type CollectedList } from '@/lib/lists'
import type {
  BatchCreateResult,
  BatchDeleteResult,
  DnsRecord,
  Group,
  Line,
  PagedDnsRecord,
} from '@/lib/types'

export interface RecordQuery {
  page?: number
  page_size?: number
  sub_domain?: string
  record_type?: string
  keyword?: string
}

/**
 * The record list is paged server side, because one DNSPod answer can be long.
 * `total` is what tells a domain with no records from one still loading.
 */
export function listRecords(
  domain: string,
  query: RecordQuery = {},
  accountId?: number
): Promise<PagedDnsRecord> {
  return request<PagedDnsRecord>({
    method: 'get',
    url: `/domains/${encodeURIComponent(domain)}/records`,
    params: { ...query, ...accountParam(accountId) },
  })
}

/**
 * Every page of a record listing, not only the first one.
 *
 * A page holds at most 200 rows, which is also the server's own cap, so a
 * domain with more than that used to be cut off silently: the table filtered
 * and paged what it had, and everything past the first page could not be seen,
 * searched or deleted. The extra calls are cheap while the server's read cache
 * is warm: it answers them from the listing it has already fetched.
 */
export function listAllRecords(
  domain: string,
  accountId?: number
): Promise<CollectedList<DnsRecord>> {
  return collectPages((page) => listRecords(domain, { page, page_size: API_PAGE_SIZE }, accountId))
}

export function listRecordLines(domain: string, accountId?: number): Promise<Line[]> {
  return request<Line[]>({
    method: 'get',
    url: `/domains/${encodeURIComponent(domain)}/lines`,
    params: accountParam(accountId),
  })
}

export function listRecordGroups(domain: string, accountId?: number): Promise<Group[]> {
  return request<Group[]>({
    method: 'get',
    url: `/domains/${encodeURIComponent(domain)}/groups`,
    params: accountParam(accountId),
  })
}

export interface RecordInput {
  sub_domain: string
  record_type: string
  value: string
  line?: string
  ttl?: number
  mx?: number
  /** Shares the answer between records of the same host and type; 0 is even. */
  weight?: number
  status?: string
  remark?: string
}

export function createRecord(
  domain: string,
  input: RecordInput,
  accountId?: number
): Promise<DnsRecord> {
  return request<DnsRecord>({
    method: 'post',
    url: `/domains/${encodeURIComponent(domain)}/records`,
    data: input,
    params: accountParam(accountId),
  })
}

export function updateRecord(
  domain: string,
  id: number,
  input: RecordInput,
  accountId?: number
): Promise<null> {
  return request<null>({
    method: 'put',
    url: `/domains/${encodeURIComponent(domain)}/records/${id}`,
    data: input,
    params: accountParam(accountId),
  })
}

export function deleteRecord(domain: string, id: number, accountId?: number): Promise<null> {
  return request<null>({
    method: 'delete',
    url: `/domains/${encodeURIComponent(domain)}/records/${id}`,
    params: accountParam(accountId),
  })
}

/** Enables or disables one record without rewriting it. */
export function setRecordStatus(
  domain: string,
  id: number,
  enabled: boolean,
  accountId?: number
): Promise<null> {
  return request<null>({
    method: 'patch',
    url: `/domains/${encodeURIComponent(domain)}/records/${id}/status`,
    data: { enable: enabled },
    params: accountParam(accountId),
  })
}

export function batchCreateRecords(
  domain: string,
  items: RecordInput[],
  accountId?: number
): Promise<BatchCreateResult> {
  return request<BatchCreateResult>({
    method: 'post',
    url: `/domains/${encodeURIComponent(domain)}/records/batch`,
    data: { items },
    params: accountParam(accountId),
  })
}

/**
 * Deletes several records at once. It is `batch-delete` rather than
 * `DELETE /records/batch` because that would be read as one more record id.
 */
export function batchDeleteRecords(
  domain: string,
  ids: number[],
  accountId?: number
): Promise<BatchDeleteResult> {
  return request<BatchDeleteResult>({
    method: 'post',
    url: `/domains/${encodeURIComponent(domain)}/records/batch-delete`,
    data: { ids },
    params: accountParam(accountId),
  })
}
