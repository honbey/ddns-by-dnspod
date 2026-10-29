import { describe, expect, it } from 'vitest'
import { formatDuration, formatTime, statusVariant } from './format'

describe('formatDuration', () => {
  it.each([
    ['missing', null, '-'],
    ['seconds', 45, '45s'],
    ['minutes', 60, '1m'],
    ['minutes and seconds', 95, '1m 35s'],
    ['hours', 3600, '1h'],
    ['hours and minutes', 7800, '2h 10m'],
    ['days', 86400, '1d'],
    ['days and hours', 129600, '1d 12h'],
  ])('renders %s as %s', (_label, input, expected) => {
    expect(formatDuration(input ?? undefined)).toBe(expected)
  })

  it('keeps zero readable as 0s', () => {
    expect(formatDuration(0)).toBe('0s')
  })
})

describe('formatTime', () => {
  it('returns a dash when there is no time', () => {
    expect(formatTime(null)).toBe('-')
  })

  it('returns the raw value rather than "Invalid Date"', () => {
    expect(formatTime('not a date')).toBe('not a date')
  })

  it('formats a real timestamp', () => {
    expect(formatTime('2026-09-19T10:20:30Z')).not.toBe('-')
  })
})

describe('statusVariant', () => {
  it.each([
    ['OK', 'default'],
    ['FAIL', 'destructive'],
    ['SKIP', 'secondary'],
    ['RUNNING', 'secondary'],
    ['', 'outline'],
  ] as const)('maps %s onto %s', (status, variant) => {
    expect(statusVariant(status)).toBe(variant)
  })
})
