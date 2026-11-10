import { describe, expect, it } from 'vitest'
import { fitsSubDomainBatch, parseSubDomains } from './subdomains'

const cases: [string, string[]][] = [
  ['home', ['home']],
  ['home www', ['home', 'www']],
  ['home,www', ['home', 'www']],
  ['home, www  @', ['home', 'www', '@']],
  ['  home   ,  ,  www  ', ['home', 'www']],
  ['@', ['@']],
]

describe('parseSubDomains', () => {
  it.each(cases)('splits %j into %j', (raw, expected) => {
    expect(parseSubDomains(raw)).toEqual(expected)
  })

  it('treats an empty field as the apex', () => {
    expect(parseSubDomains('   ')).toEqual(['@'])
  })

  it('drops a host repeated in the same field', () => {
    expect(parseSubDomains('a a,b a')).toEqual(['a', 'b'])
  })

  it('names the batch limit', () => {
    expect(fitsSubDomainBatch('a b')).toBe(true)
    const tooMany = Array.from({ length: 101 }, (_, i) => `h${i}`).join(' ')
    expect(fitsSubDomainBatch(tooMany)).toBe(false)
  })
})
