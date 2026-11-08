/**
 * The HTTP kernel: one axios instance, one error shape, one place the bearer
 * token is attached.
 *
 * Business endpoints do not belong here: they live beside the feature they
 * serve, in `features/<name>/api.ts`.
 */
import axios, { type AxiosRequestConfig } from 'axios'
import type { components } from '@/api-types.gen'
import { authRequired } from '@/lib/config'
import { getAccessToken } from '@/lib/auth'

interface Envelope<T> {
  data: T
}

/** Query parameters, which every listing here builds the same way. */
export type Query = Record<string, string | number | boolean | undefined>

/** The error codes the contract defines; a new one in openapi.yml appears here. */
type ErrorCode = NonNullable<components['schemas']['Error']['code']>

/**
 * The axios instance the kernel owns. Exported so a test can install an
 * adapter; application code goes through `request` instead.
 */
export const http = axios.create({ baseURL: '/api', timeout: 30_000 })

/**
 * The base came from `config.json` rather than being compiled in, so one build
 * can be served from somewhere other than `/api`.
 */
export function setApiBaseUrl(url: string): void {
  http.defaults.baseURL = url || '/api'
}

http.interceptors.request.use(async (config) => {
  // With verification off the server accepts every request, so asking for a
  // token would only delay it.
  if (!authRequired()) return config

  const token = await getAccessToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

/** A failed call, already reduced to what the UI needs. */
export class ApiError extends Error {
  readonly status: number
  readonly requestId: string | undefined
  /**
   * A stable identifier for the failures a client reacts to instead of
   * showing, e.g. `no_account`. Absent unless the server named the condition:
   * the message is what a person reads, this is what code branches on.
   */
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
  // A network failure or timeout has no server wording worth showing - the
  // axios text is English and internal. Leaving the message empty lets the
  // caller's fallback speak.
  const message =
    body?.error ?? (status === 0 || error.code === 'ECONNABORTED' ? '' : error.message)
  return new ApiError(message, status, { requestId: body?.request_id, code: body?.code })
}

/**
 * Runs a request and unwraps the server's `{data: ...}` envelope. Callers see
 * values, never envelopes or axios internals.
 *
 * The signal is what lets a query cancellation actually cancel the request:
 * without it an abandoned page keeps loading and its answer lands in a cache
 * nobody is looking at.
 *
 * A body-less answer (204) leaves axios with an empty string where `data`
 * should be, so the declared `null` is restored before the caller sees it: a
 * signature that promises `null` has to hand back something `=== null`
 * answers to, not `undefined`.
 */
export async function request<T>(config: AxiosRequestConfig, signal?: AbortSignal): Promise<T> {
  try {
    const response = await http.request<Envelope<T>>({ ...config, signal })
    const payload = response.data.data
    return (payload ?? null) as T
  } catch (error) {
    // A cancelled request is not a failure of the API: it is the caller
    // walking away, and reporting it would toast on every navigation.
    if (axios.isCancel(error)) throw error
    throw toApiError(error)
  }
}
