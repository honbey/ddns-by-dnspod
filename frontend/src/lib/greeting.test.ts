import { describe, expect, it } from 'vitest'
import { greetingKey } from './greeting'

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
