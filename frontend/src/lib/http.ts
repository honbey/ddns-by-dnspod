import axios, { type AxiosRequestConfig } from 'axios'
import type { components } from '@/api-types.gen'
import { authRequired } from '@/lib/config'
import { getAccessToken } from '@/lib/auth'

interface Envelope<T> {
  data: T
}

export type Query = Record<string, string | number | boolean | undefined>

/** Error codes are the enum in openapi.yml. */
type ErrorCode = NonNullable<components['schemas']['Error']['code']>

/** Exported so a test can install an adapter. */
export const http = axios.create({ baseURL: '/api', timeout: 30_000 })

export function setApiBaseUrl(url: string): void {
  http.defaults.baseURL = url || '/api'
}

http.interceptors.request.use(async (config) => {
  if (!authRequired()) return config
  const token = await getAccessToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

export class ApiError extends Error {
  readonly status: number
  readonly requestId: string | undefined
  readonly code: ErrorCode | undefined

  constructor(
    message: string,
    status: number,
    options: { requestId?: string; code?: ErrorCode } = {}
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.requestId = options.requestId
    this.code = options.code
  }
}

function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  if (!axios.isAxiosError(error)) {
    return new ApiError(error instanceof Error ? error.message : 'request failed', 0)
  }

  const status = error.response?.status ?? 0
  const body = error.response?.data as components['schemas']['Error'] | undefined
  // Network failures have no server wording; leave the message empty for the caller's fallback.
  const message =
    body?.error ?? (status === 0 || error.code === 'ECONNABORTED' ? '' : error.message)
  return new ApiError(message, status, { requestId: body?.request_id, code: body?.code })
}

/** Unwraps `{data: ...}`; a 204's empty string becomes null. */
export async function request<T>(config: AxiosRequestConfig, signal?: AbortSignal): Promise<T> {
  try {
    const response = await http.request<Envelope<T>>({ ...config, signal })
    const payload = response.data.data
    return (payload ?? null) as T
  } catch (error) {
    // A cancelled request is the caller walking away, not an API failure.
    if (axios.isCancel(error)) throw error
    throw toApiError(error)
  }
}
