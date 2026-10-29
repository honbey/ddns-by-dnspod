import { describe, expect, it } from 'vitest'
import { expiryOf, formatNames, parseNames } from './certificate-types'
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
    auto_renew: true,
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
    expect(expiryOf(certificate(days(62))).label).toBe('62 days left')
    expect(expiryOf(certificate(days(1))).label).toBe('1 day left')
  })

  it('turns the badge when the renewal should have happened', () => {
    expect(expiryOf(certificate(days(62))).variant).toBe('default')
    expect(expiryOf(certificate(days(20))).variant).toBe('secondary')
    expect(expiryOf(certificate(days(3))).variant).toBe('destructive')
  })

  it('says when one has expired', () => {
    const expired = expiryOf(certificate(days(-1)))
    expect(expired.label).toBe('expired')
    expect(expired.variant).toBe('destructive')
  })

  it('says when one was never issued', () => {
    const missing = expiryOf(certificate(null))
    expect(missing.label).toBe('not issued')
    expect(missing.daysLeft).toBeNull()
  })

  it('survives a value that is not a date', () => {
    // What a hand-edited database row looks like; the table still has to render.
    expect(expiryOf(certificate('whenever')).label).toBe('-')
  })
})
