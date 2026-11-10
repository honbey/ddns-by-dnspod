import { describe, expect, it } from 'vitest'
import { DEFAULT_THEME, parseTheme } from './theme'

describe('parseTheme', () => {
  it('keeps every value of the enum', () => {
    for (const value of ['dark', 'light', 'system']) {
      expect(parseTheme(value)).toBe(value)
    }
  })

  it('falls back when a stored value is not a preference', () => {
    for (const value of [null, undefined, '', 'foo', 'DARK', 0, {}, []]) {
      expect(parseTheme(value)).toBe(DEFAULT_THEME)
    }
  })

  it('honours the caller fallback', () => {
    expect(parseTheme('', 'dark')).toBe('dark')
  })
})
