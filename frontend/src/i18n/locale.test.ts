import { describe, expect, it } from 'vitest'
import { createEpochGate, preferenceFromStorage, resolveLocale } from './locale'

describe('resolveLocale', () => {
  it('follows the first browser language when nothing is stored', () => {
    expect(resolveLocale(null, ['en-US', 'zh-CN'])).toBe('en')
    expect(resolveLocale(null, ['zh-CN'])).toBe('zh')
    expect(resolveLocale(null, [])).toBe('zh')
  })

  it('falls back to Chinese for a third language', () => {
    expect(resolveLocale(null, ['fr-FR'])).toBe('zh')
  })

  it('lets an explicit choice override the browser', () => {
    expect(resolveLocale('zh', ['en-US'])).toBe('zh')
    expect(resolveLocale('en', ['zh-CN'])).toBe('en')
  })

  it('treats an unknown stored value as following the system', () => {
    expect(resolveLocale('system', ['en-GB'])).toBe('en')
    expect(resolveLocale('garbage', ['zh-CN'])).toBe('zh')
  })
})

describe('preferenceFromStorage', () => {
  it('keeps an explicit choice from the event value', () => {
    expect(preferenceFromStorage('en')).toBe('en')
    expect(preferenceFromStorage('zh')).toBe('zh')
  })

  it('treats a cleared, system or unknown value as following the system', () => {
    expect(preferenceFromStorage(null)).toBe('system')
    expect(preferenceFromStorage('system')).toBe('system')
    expect(preferenceFromStorage('garbage')).toBe('system')
  })
})

describe('createEpochGate', () => {
  it('accepts only the most recent request', () => {
    const gate = createEpochGate()
    const first = gate.next()
    const second = gate.next()

    expect(gate.isCurrent(first)).toBe(false)
    expect(gate.isCurrent(second)).toBe(true)

    const third = gate.next()
    expect(gate.isCurrent(second)).toBe(false)
    expect(gate.isCurrent(third)).toBe(true)
  })
})
