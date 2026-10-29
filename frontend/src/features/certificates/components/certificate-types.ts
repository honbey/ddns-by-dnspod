import { type Certificate } from '@/lib/types'

/**
 * What the server can be asked to issue. The list is the backend's
 * acme.KeyTypes, in the order the select offers them: P-256 first, because it
 * is what a deployment that has a choice picks.
 */
export const KEY_TYPES = [
  { id: 'ec256', label: 'ECDSA P-256', hint: 'the default, and the smallest' },
  { id: 'ec384', label: 'ECDSA P-384', hint: 'a stronger curve, slower to use' },
  { id: 'rsa2048', label: 'RSA 2048', hint: 'for a peer that cannot do ECDSA' },
  { id: 'rsa4096', label: 'RSA 4096', hint: 'the same, with a long key' },
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

type Expiry = {
  /** How long is left, in words; "-" when there is no certificate yet. */
  label: string
  /** Whether it is close enough to be a problem worth colouring. */
  variant: 'default' | 'secondary' | 'destructive' | 'outline'
  daysLeft: number | null
}

/**
 * How long a certificate has left, from the notAfter the CA set.
 *
 * The thresholds are the ones an operator acts on: a month is when a renewal
 * that is not happening becomes worth looking at, a week is when it is late.
 */
export function expiryOf(cert: Certificate): Expiry {
  if (!cert.not_after) {
    return { label: 'not issued', variant: 'outline', daysLeft: null }
  }
  const notAfter = new Date(cert.not_after)
  if (Number.isNaN(notAfter.getTime())) {
    return { label: '-', variant: 'outline', daysLeft: null }
  }

  const daysLeft = Math.floor((notAfter.getTime() - Date.now()) / 86_400_000)
  if (daysLeft < 0) {
    return { label: 'expired', variant: 'destructive', daysLeft }
  }
  if (daysLeft === 0) {
    return { label: 'expires today', variant: 'destructive', daysLeft }
  }

  return {
    label: `${daysLeft} day${daysLeft === 1 ? '' : 's'} left`,
    variant: daysLeft < 7 ? 'destructive' : daysLeft < 30 ? 'secondary' : 'default',
    daysLeft,
  }
}
