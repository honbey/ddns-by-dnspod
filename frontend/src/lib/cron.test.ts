import { type TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'
import {
  certificateCronPresets,
  describeCron,
  isValidCron,
  RECORD_TYPES,
  taskCronPresets,
  TTL_OPTIONS,
} from './cron'

// The dictionary is not loaded here: what the mapping decides is which key it
// asks for, and that key is enough to see the answer.
const t = ((key: string) => key) as TFunction

describe('describeCron', () => {
  it.each([...taskCronPresets, ...certificateCronPresets(3)])(
    'names $value by its key',
    ({ value, key }) => {
      expect(describeCron(value, t)).toBe(key)
    }
  )

  it('names a certificate schedule whatever hour it runs at', () => {
    expect(describeCron('0 5 * * *', t)).toBe('cron.daily')
    expect(describeCron('0 23 */7 * *', t)).toBe('cron.every7Days')
    expect(describeCron('0 7 1 * *', t)).toBe('cron.monthly')
  })

  it('leaves a schedule that only looks like one as written', () => {
    // Same day-of-month, but not on the hour: not the shortcut's shape.
    expect(describeCron('30 5 * * *', t)).toBe('30 5 * * *')
    expect(describeCron('0 24 * * *', t)).toBe('0 24 * * *')
  })

  it('shows anything else as written rather than guessing', () => {
    expect(describeCron('*/7 2 * * 1', t)).toBe('*/7 2 * * 1')
  })

  it('ignores surrounding whitespace when matching a preset', () => {
    expect(describeCron('  */5 * * * *  ', t)).toBe('cron.every5Minutes')
  })

  it('reads a step of one as the wildcard it behaves like', () => {
    expect(describeCron('*/1 * * * *', t)).toBe('cron.everyMinute')
    expect(describeCron('*/01 * * * *', t)).toBe('cron.everyMinute')
    expect(describeCron('0 */1 * * *', t)).toBe('cron.hourly')
  })

  it('collapses extra spaces between fields', () => {
    expect(describeCron('*/5   * * * *', t)).toBe('cron.every5Minutes')
    expect(describeCron('*/1   * * * *', t)).toBe('cron.everyMinute')
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
    expect(TTL_OPTIONS.every((option) => option.key.length > 0)).toBe(true)
  })
})
