/** API keys for the public REST API under /api/v1. */
import { request } from '@/lib/http'
import { API_PAGE_SIZE, collectPages } from '@/lib/lists'
import { getConfig } from '@/lib/config'
import type { APIKey, APIKeyCreated, PagedAPIKey } from '@/lib/types'

/**
 * The scope of a key that may touch every domain of its account rather than one
 * name. The server stores this value verbatim, so it is part of the wire
 * contract: see model.APIKeyWildcard in the backend.
 */
export const API_KEY_ANY_DOMAIN = '*'

export interface APIKeyInput {
  name: string
  domain: string
  account_id: number
  enabled?: boolean
}

/** Every key; the table filters client side, so it reads the pages through. */
export function listAPIKeys(): Promise<APIKey[]> {
  return collectPages((page) =>
    request<PagedAPIKey>({
      method: 'get',
      url: '/keys',
      params: { page, page_size: API_PAGE_SIZE },
    })
  ).then((list) => list.items)
}

/**
 * Creates a key. The plaintext comes back here and nowhere else: only a prefix
 * is stored, so a later listing cannot reproduce it.
 */
export function createAPIKey(input: APIKeyInput): Promise<APIKeyCreated> {
  return request<APIKeyCreated>({ method: 'post', url: '/keys', data: input })
}

export function updateAPIKey(id: number, input: Partial<APIKeyInput>): Promise<APIKey> {
  return request<APIKey>({ method: 'put', url: `/keys/${id}`, data: input })
}

export function deleteAPIKey(id: number): Promise<null> {
  return request<null>({ method: 'delete', url: `/keys/${id}` })
}

/** Where the public REST API is reachable, for the copyable examples. */
export function publicApiBase(): string {
  const configured = getConfig().apiBaseUrl.trim()
  if (/^https?:\/\//i.test(configured)) return configured
  return `${window.location.origin}${configured.startsWith('/') ? '' : '/'}${configured}`
}
