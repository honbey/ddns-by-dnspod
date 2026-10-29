import { describe, expect, it } from 'vitest'
import { cronPresets, describeCron, isValidCron, RECORD_TYPES, TTL_OPTIONS } from './cron'

describe('describeCron', () => {
  it.each(cronPresets)('names $value in words', ({ value, label }) => {
    expect(describeCron(value)).toBe(label)
  })

  it('shows anything else as written rather than guessing', () => {
    expect(describeCron('*/7 2 * * 1')).toBe('*/7 2 * * 1')
  })

  it('ignores surrounding whitespace when matching a preset', () => {
    expect(describeCron('  */5 * * * *  ')).toBe('Every 5 minutes')
  })
})

// The dialog has to agree with the scheduler: telling someone a schedule is
// fine and then having the server refuse it only produces a confusing error.
describe('isValidCron', () => {
  it.each([
    '* * * * *',
    '*/5 * * * *',
    '0 3 * * *',
    '0 */6 * * *',
    '30 2 1,15 * *',
    '0 0 * oct *',
    '0 0 * * mon-fri',
    '@daily',
    '@every 1h30m',
  ])('accepts %s', (expr) => {
    expect(isValidCron(expr)).toBe(true)
  })

  it.each([
    '99 * * * *',
    '*/0 * * * *',
    '5-1 * * * *',
    '* * * * * *',
    '* * * *',
    '0 25 * * *',
    '@weekly-ish',
    '@every',
  ])('rejects %s', (expr) => {
    expect(isValidCron(expr)).toBe(false)
  })
})

describe('the option lists', () => {
  it('offers only the record types DDNS can follow', () => {
    expect([...RECORD_TYPES]).toEqual(['A', 'AAAA'])
  })

  it('labels every TTL in words, in ascending order', () => {
    const values = TTL_OPTIONS.map((option) => option.value)
    expect(values).toEqual([...values].sort((a, b) => a - b))
    expect(TTL_OPTIONS.every((option) => option.label.length > 0)).toBe(true)
  })
})
