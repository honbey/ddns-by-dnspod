import { describe, expect, it } from 'vitest'
import { fitsSubDomainBatch, hostOf, parseSubDomains } from './subdomains'

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

describe('hostOf', () => {
  it('puts the subdomain in front of the domain', () => {
    expect(hostOf({ sub_domain: 'home', domain: 'example.com' })).toBe('home.example.com')
  })

  it('reads the apex as the domain itself', () => {
    // '@' is what the server stores for a record at the apex.
    expect(hostOf({ sub_domain: '@', domain: 'example.com' })).toBe('example.com')
    expect(hostOf({ sub_domain: '', domain: 'example.com' })).toBe('example.com')
  })
})
