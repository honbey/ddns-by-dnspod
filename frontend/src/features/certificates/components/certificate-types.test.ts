import { describe, expect, it } from 'vitest'
import { type TFunction } from 'i18next'
import { expiryLabel, expiryOf, formatNames, parseNames, type Expiry } from './certificate-types'
import { type Certificate } from '@/lib/types'

function certificate(notAfter: string | null): Certificate {
  return {
    id: 1,
    name: 'site',
    account_id: 1,
    domains: ['example.com'],
    key_type: 'ec256',
    output_dir: '',
    output_path: '/certs/site',
    renew_cron: '0 3 * * *',
    renew_before_days: 0,
    notify_on_issue: true,
    last_issued_at: null,
    not_after: notAfter,
    last_status: 'OK',
    last_message: '',
    fail_count: 0,
    created_at: '2026-10-15T00:00:00Z',
    updated_at: '2026-10-15T00:00:00Z',
  }
}

// Half a day past the boundary the label names, so the assertion does not
// depend on how long the test takes to reach it.
const days = (count: number) => new Date(Date.now() + (count + 0.5) * 86_400_000).toISOString()

describe('parseNames', () => {
  it('reads one name per line', () => {
    expect(parseNames('example.com\nwww.example.com')).toEqual(['example.com', 'www.example.com'])
  })

  it('reads a list pasted from anywhere', () => {
    expect(parseNames(' example.com, www.example.com ,*.example.com ')).toEqual([
      'example.com',
      'www.example.com',
      '*.example.com',
    ])
  })

  it('drops blank lines', () => {
    expect(parseNames('\n\n  \n')).toEqual([])
  })

  it('lowercases, because a name is one whatever the case', () => {
    expect(parseNames('EXAMPLE.com')).toEqual(['example.com'])
  })

  it('round trips through the form', () => {
    const names = ['example.com', '*.example.com']
    expect(parseNames(formatNames(names))).toEqual(names)
  })
})

describe('expiryOf', () => {
  it('counts the days left', () => {
    expect(expiryOf(certificate(days(62)))).toMatchObject({ kind: 'days', daysLeft: 62 })
    expect(expiryOf(certificate(days(1)))).toMatchObject({ kind: 'days', daysLeft: 1 })
  })

  it('turns the badge when the renewal should have happened', () => {
    expect(expiryOf(certificate(days(62))).variant).toBe('default')
    expect(expiryOf(certificate(days(20))).variant).toBe('secondary')
    expect(expiryOf(certificate(days(3))).variant).toBe('destructive')
  })

  it('says when one has expired', () => {
    const expired = expiryOf(certificate(days(-1)))
    expect(expired.kind).toBe('expired')
    expect(expired.variant).toBe('destructive')
  })

  it('says when one was never requested', () => {
    const missing = expiryOf(certificate(null))
    expect(missing.kind).toBe('not-requested')
    expect(missing.daysLeft).toBeNull()
  })

  it('survives a value that is not a date', () => {
    // What a hand-edited database row looks like; the table still has to render.
    expect(expiryOf(certificate('whenever')).kind).toBe('unreadable')
  })
})

describe('expiryLabel', () => {
  // Stands in for i18next: the key and the count are what the mapping decides.
  const t = ((key: string, options?: { count?: number }) =>
    options?.count === undefined ? key : `${key}#${options.count}`) as TFunction

  const label = (expiry: Expiry) => expiryLabel(expiry, t)

  it('names the day count for the dictionary to pluralise', () => {
    expect(label({ kind: 'days', variant: 'default', daysLeft: 62 })).toBe(
      'certificates.expiresIn#62'
    )
  })

  it('names every other kind', () => {
    expect(label({ kind: 'not-requested', variant: 'outline', daysLeft: null })).toBe(
      'certificates.notRequested'
    )
    expect(label({ kind: 'expired', variant: 'destructive', daysLeft: -1 })).toBe(
      'certificates.expired'
    )
    expect(label({ kind: 'today', variant: 'destructive', daysLeft: 0 })).toBe(
      'certificates.expiresToday'
    )
    // Nothing to translate: the date itself is unreadable.
    expect(label({ kind: 'unreadable', variant: 'outline', daysLeft: null })).toBe('-')
  })
})

// The union is the guard, not the label: a day count cannot be missing from a
// kind that counts days. This line fails to compile if that ever widens.
// @ts-expect-error a day count is a number, not null
const countedButEmpty: Expiry = { kind: 'days', variant: 'default', daysLeft: null }
void countedButEmpty
