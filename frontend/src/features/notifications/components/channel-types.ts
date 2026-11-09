import { BellRing, Webhook } from 'lucide-react'
import { type NotifyChannel } from '@/lib/types'

/** ntfy, as the notifier reads it. */
export interface NtfyConfig {
  server?: string
  topic?: string
  username?: string
  password?: string
  priority?: number
  tags?: string
  skip_tls_verify?: boolean
  title_template?: string
  body_template?: string
}

/** A generic webhook, whose body is rendered as a Go template. */
export interface WebhookConfig {
  url?: string
  method?: string
  headers?: Record<string, string>
  body?: string
  skip_tls_verify?: boolean
}

export const CHANNEL_TYPES = [
  { id: 'ntfy', label: 'ntfy', icon: BellRing, hint: 'publishes to a topic' },
  { id: 'webhook', label: 'Webhook', icon: Webhook, hint: 'posts JSON or a template' },
] as const

export type ChannelType = (typeof CHANNEL_TYPES)[number]['id']

/** The label to show for a stored type; unknown types are shown as stored. */
export function channelTypeLabel(type: string): string {
  return CHANNEL_TYPES.find((item) => item.id === type)?.label ?? type
}

/**
 * The stored configuration, parsed.
 *
 * It arrives as one JSON string because that is what keeps two different shapes
 * in one column; a malformed one is treated as empty rather than thrown away,
 * because a broken config still has to be editable to be fixed.
 */
export function parseConfig(channel: NotifyChannel): Record<string, unknown> {
  try {
    const parsed = JSON.parse(channel.config) as unknown
    return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

/** Where the message goes, which is what identifies a channel at a glance. */
export function channelTarget(channel: NotifyChannel): string {
  const config = parseConfig(channel)
  if (channel.type === 'ntfy') {
    const server = String(config.server ?? '')
    const topic = String(config.topic ?? '')
    return topic ? `${server.replace(/\/$/, '')}/${topic}` : server || '-'
  }
  return String(config.url ?? '-')
}

/** A stored headers object as editable rows, in insertion order. */
export function headersOf(channel: NotifyChannel): { name: string; value: string }[] {
  return Object.entries(parseConfig(channel).headers ?? {}).map(([name, value]) => ({
    name,
    value: String(value),
  }))
}

export function ntfyOf(channel: NotifyChannel): NtfyConfig {
  const config = parseConfig(channel)
  return {
    server: config.server as string | undefined,
    topic: config.topic as string | undefined,
    username: config.username as string | undefined,
    priority: typeof config.priority === 'number' ? config.priority : 3,
    tags: (config.tags as string | undefined) ?? '',
    skip_tls_verify: Boolean(config.skip_tls_verify),
    title_template: (config.title_template as string | undefined) ?? '',
    body_template: (config.body_template as string | undefined) ?? '',
  }
}

export function webhookOf(channel: NotifyChannel): WebhookConfig {
  const config = parseConfig(channel)
  return {
    url: config.url as string | undefined,
    method: (config.method as string | undefined) ?? 'POST',
    body: (config.body as string | undefined) ?? '',
    skip_tls_verify: Boolean(config.skip_tls_verify),
  }
}
