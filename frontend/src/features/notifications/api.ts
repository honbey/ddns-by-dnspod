/** Notification channels: where a change or a failure is announced. */
import { request } from '@/lib/http'
import { API_PAGE_SIZE, collectPages } from '@/lib/lists'
import type { NotifyChannel, PagedNotifyChannel } from '@/lib/types'

/** The configuration is a JSON object whose shape depends on the type. */
export type ChannelConfig = Record<string, unknown>

export interface ChannelInput {
  name: string
  type: string
  enabled?: boolean
  config: ChannelConfig
}

/** Every channel; the table filters client side, so it reads the pages through. */
export function listChannels(): Promise<NotifyChannel[]> {
  return collectPages((page) =>
    request<PagedNotifyChannel>({
      method: 'get',
      url: '/notify/channels',
      params: { page, page_size: API_PAGE_SIZE },
    })
  ).then((list) => list.items)
}

export function createChannel(input: ChannelInput): Promise<NotifyChannel> {
  return request<NotifyChannel>({ method: 'post', url: '/notify/channels', data: input })
}

export function updateChannel(id: number, input: ChannelInput): Promise<NotifyChannel> {
  return request<NotifyChannel>({
    method: 'put',
    url: `/notify/channels/${id}`,
    data: input,
  })
}

export function deleteChannel(id: number): Promise<null> {
  return request<null>({ method: 'delete', url: `/notify/channels/${id}` })
}

/** Delivers one throwaway message, which is the only way to know it works. */
export function testChannel(id: number): Promise<null> {
  return request<null>({ method: 'post', url: `/notify/channels/${id}/test` })
}
