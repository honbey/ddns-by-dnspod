import { type TFunction } from 'i18next'
import { type Certificate } from '@/lib/types'

/**
 * What the server can be asked to request. The list is the backend's
 * acme.KeyTypes, in the order the select offers them: P-256 first, because it
 * is what a deployment that has a choice picks.
 */
export const KEY_TYPES = [
  { id: 'ec256', label: 'ECDSA P-256' },
  { id: 'ec384', label: 'ECDSA P-384' },
  { id: 'rsa2048', label: 'RSA 2048' },
  { id: 'rsa4096', label: 'RSA 4096' },
] as const

export function keyTypeLabel(keyType: string): string {
  return KEY_TYPES.find((item) => item.id === keyType)?.label ?? keyType
}

/**
 * The names of a certificate, as one editable block.
 *
 * One name per line is what the textarea shows and what this reads back: a
 * paste of a comma or space separated list is split the same way, because that
 * is how the list arrives from wherever it was copied.
 */
export function parseNames(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((name) => name.trim().toLowerCase())
    .filter((name) => name.length > 0)
}

export function formatNames(names: string[]): string {
  return names.join('\n')
}

/** Whether it is close enough to be a problem worth colouring. */
type ExpiryVariant = 'default' | 'secondary' | 'destructive' | 'outline'

/**
 * How long a certificate has left, as the cell sees it.
 *
 * The kinds that count days carry the count, the ones that do not carry null:
 * so a count can never be read off a kind that has none, nor be missing from
 * one that does.
 */
export type Expiry =
  | { kind: 'days'; variant: ExpiryVariant; daysLeft: number }
  | { kind: 'expired'; variant: ExpiryVariant; daysLeft: number }
  | { kind: 'today'; variant: ExpiryVariant; daysLeft: number }
  | { kind: 'not-requested'; variant: ExpiryVariant; daysLeft: null }
  | { kind: 'unreadable'; variant: ExpiryVariant; daysLeft: null }

/**
 * How long a certificate has left, from the notAfter the CA set.
 *
 * The thresholds are the ones an operator acts on: a month is when a renewal
 * that is not happening becomes worth looking at, a week is when it is late.
 * The wording itself lives in the dictionary, so this stays a pure function.
 */
export function expiryOf(cert: Certificate): Expiry {
  if (!cert.not_after) {
    return { kind: 'not-requested', variant: 'outline', daysLeft: null }
  }
  const notAfter = new Date(cert.not_after)
  if (Number.isNaN(notAfter.getTime())) {
    return { kind: 'unreadable', variant: 'outline', daysLeft: null }
  }

  const daysLeft = Math.floor((notAfter.getTime() - Date.now()) / 86_400_000)
  if (daysLeft < 0) {
    return { kind: 'expired', variant: 'destructive', daysLeft }
  }
  if (daysLeft === 0) {
    return { kind: 'today', variant: 'destructive', daysLeft }
  }

  return {
    kind: 'days',
    variant: daysLeft < 7 ? 'destructive' : daysLeft < 30 ? 'secondary' : 'default',
    daysLeft,
  }
}

/**
 * The badge wording for an expiry.
 *
 * Every kind is spelled out rather than defaulted, so a new one fails to
 * compile instead of rendering as a dash nobody notices.
 */
export function expiryLabel(expiry: Expiry, t: TFunction): string {
  switch (expiry.kind) {
    case 'days':
      return t('certificates.expiresIn', { count: expiry.daysLeft })
    case 'not-requested':
      return t('certificates.notRequested')
    case 'expired':
      return t('certificates.expired')
    case 'today':
      return t('certificates.expiresToday')
    case 'unreadable':
      // Nothing truthful to say about a notAfter that is not a date.
      return '-'
  }
}
