import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { greetingKey, msUntilNextHour } from './greeting'

describe('greetingKey', () => {
  it.each([
    [0, 'night'],
    [4, 'night'],
    [5, 'morning'],
    [11, 'morning'],
    [12, 'afternoon'],
    [17, 'afternoon'],
    [18, 'evening'],
    [22, 'evening'],
    [23, 'night'],
  ])('greets hour %i as %s', (hour, period) => {
    expect(greetingKey(hour)).toBe(`dashboard.greeting.${period}`)
  })

  it('names a greeting for every hour of the day', () => {
    for (let hour = 0; hour < 24; hour += 1) {
      expect(greetingKey(hour)).toMatch(/^dashboard\.greeting\./)
    }
  })
})

describe('msUntilNextHour', () => {
  it('lands exactly on the next whole hour', () => {
    const now = new Date(2026, 10, 10, 11, 30, 15, 250)
    const expected = new Date(2026, 10, 10, 12, 0, 0, 0).getTime() - now.getTime()
    expect(msUntilNextHour(now)).toBe(expected)
  })

  it('crosses midnight into the next day', () => {
    const now = new Date(2026, 10, 10, 23, 59, 0, 0)
    const next = new Date(now.getTime() + msUntilNextHour(now))
    expect([next.getDate(), next.getHours(), next.getMinutes()]).toEqual([11, 0, 0])
  })

  it('waits a full hour from an exact boundary', () => {
    const now = new Date(2026, 10, 10, 12, 0, 0, 0)
    expect(msUntilNextHour(now)).toBe(60 * 60_000)
  })

  it('stays positive one millisecond past a boundary', () => {
    const now = new Date(2026, 10, 10, 12, 0, 0, 1)
    expect(msUntilNextHour(now)).toBe(60 * 60_000 - 1)
  })

  it('approaches zero one millisecond before the next boundary', () => {
    const now = new Date(2026, 10, 10, 12, 59, 59, 999)
    expect(msUntilNextHour(now)).toBe(1)
  })

  it('waits a whole hour when the clock steps back past the boundary', () => {
    const now = new Date(2026, 10, 10, 12, 30, 0, 0)
    // A DST fold or an NTP correction can put the computed boundary behind
    // `now`. Force that branch so the recursive timer cannot spin at 1ms.
    const setHours = vi.spyOn(Date.prototype, 'setHours').mockImplementation(function (this: Date) {
      return this.setTime(now.getTime() - 1)
    })
    try {
      expect(msUntilNextHour(now)).toBe(60 * 60_000)
    } finally {
      setHours.mockRestore()
    }
  })
})

// The comment in greeting.ts claims setHours stays exact across a DST step.
// Pin the timezone and the two 2026 transitions so that claim cannot rot.
describe('msUntilNextHour across a DST step', () => {
  let originalTZ: string | undefined

  beforeAll(() => {
    originalTZ = process.env.TZ
    process.env.TZ = 'America/New_York'
  })

  afterAll(() => {
    if (originalTZ === undefined) delete process.env.TZ
    else process.env.TZ = originalTZ
  })

  it('lands on the first hour that exists when the clock springs forward', () => {
    const now = new Date(2026, 2, 8, 1, 30, 0, 0)
    const next = new Date(now.getTime() + msUntilNextHour(now))
    expect(msUntilNextHour(now)).toBe(30 * 60_000)
    expect([next.getHours(), next.getMinutes()]).toEqual([3, 0])
  })

  it('waits through the repeated hour when the clock falls back', () => {
    const now = new Date(2026, 10, 1, 1, 30, 0, 0)
    const next = new Date(now.getTime() + msUntilNextHour(now))
    expect(msUntilNextHour(now)).toBe(90 * 60_000)
    expect([next.getHours(), next.getMinutes()]).toEqual([2, 0])
  })
})
