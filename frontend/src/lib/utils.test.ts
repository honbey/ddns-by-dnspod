import { describe, expect, it } from 'vitest'
import { isNavigationTarget, parsePageTarget } from './utils'

describe('parsePageTarget', () => {
  it('keeps the current page for blanks, zeros and garbage', () => {
    expect(parsePageTarget('', 3, 10)).toBe(3)
    expect(parsePageTarget('0', 3, 10)).toBe(3)
    expect(parsePageTarget('-2', 3, 10)).toBe(3)
    expect(parsePageTarget('abc', 3, 10)).toBe(3)
  })

  it('clamps values past the last page', () => {
    expect(parsePageTarget('11', 3, 10)).toBe(10)
    // parseInt returns a value past Number.MAX_SAFE_INTEGER here.
    expect(parsePageTarget('999999999999999999999', 3, 10)).toBe(10)
  })

  it('keeps values inside the range', () => {
    expect(parsePageTarget('5', 1, 10)).toBe(5)
    expect(parsePageTarget('1', 4, 10)).toBe(1)
  })

  // A table with no rows still has one page; an index of -1 is not a page.
  it('never returns a page below one', () => {
    expect(parsePageTarget('5', 3, 0)).toBe(1)
    expect(parsePageTarget('', 0, 0)).toBe(1)
    expect(parsePageTarget('2', 0, 5)).toBe(2)
  })

  // parseInt is lenient; the input already strips non-digits, and this records
  // what the exported helper does on its own.
  it('reads a leading number and ignores the rest', () => {
    expect(parsePageTarget('1abc', 3, 10)).toBe(1)
  })
})

describe('isNavigationTarget', () => {
  it('is true only when focus moved inside the navigation group', () => {
    const navigation = document.createElement('div')
    const button = document.createElement('button')
    navigation.append(button)
    const outside = document.createElement('input')

    expect(isNavigationTarget(button, navigation)).toBe(true)
    expect(isNavigationTarget(outside, navigation)).toBe(false)
    expect(isNavigationTarget(null, navigation)).toBe(false)
    expect(isNavigationTarget(button, null)).toBe(false)
  })
})
