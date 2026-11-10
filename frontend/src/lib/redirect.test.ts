import { describe, expect, it } from 'vitest'
import { returnPathOf, safeReturnPath } from './redirect'

describe('safeReturnPath', () => {
  it('keeps a path on this origin', () => {
    expect(safeReturnPath('/tasks')).toBe('/tasks')
    expect(safeReturnPath('/settings/appearance?tab=layout')).toBe(
      '/settings/appearance?tab=layout'
    )
  })

  it('refuses anything that leaves the origin', () => {
    expect(safeReturnPath('//evil.example')).toBe('/')
    expect(safeReturnPath('https://evil.example')).toBe('/')
    expect(safeReturnPath('/\\evil.example')).toBe('/')
    expect(safeReturnPath('javascript:alert(1)')).toBe('/')
  })

  it('strips control characters before deciding', () => {
    // Browsers drop tab and newline while parsing, so these are "//" in
    // disguise - and a naive prefix check would let them through.
    expect(safeReturnPath('/\t/evil.example')).toBe('/')
    expect(safeReturnPath('/\n/evil.example')).toBe('/')
    expect(safeReturnPath('\t//evil.example')).toBe('/')
    expect(safeReturnPath('/ta\tsks')).toBe('/tasks')
  })

  it('strips the C1 block too, though no browser treats it as a separator', () => {
    // Measured in chromium: `new URL('/\u0085/evil.example')` stays on the
    // origin rather than becoming protocol-relative, so the block is not a
    // bypass. It is removed anyway, and what remains is judged like any other
    // path: here that leaves a "//" in disguise, which is refused.
    expect(safeReturnPath('/\u0085/evil.example')).toBe('/')
    expect(safeReturnPath('\u0085//evil.example')).toBe('/')
    // The same removal inside a path keeps the path and drops the character.
    expect(safeReturnPath('/ta\u0085sks')).toBe('/tasks')
  })

  it('falls back for a value that is not a string', () => {
    expect(safeReturnPath(undefined)).toBe('/')
    expect(safeReturnPath(null)).toBe('/')
    expect(safeReturnPath(42)).toBe('/')
    expect(safeReturnPath('')).toBe('/')
  })
})

describe('returnPathOf', () => {
  it('keeps a path that is not the sign-in page', () => {
    expect(returnPathOf('/accounts')).toBe('/accounts')
    expect(returnPathOf('/history?page=2&status=FAIL')).toBe('/history?page=2&status=FAIL')
  })

  it('unwraps sign-in redirects, however deep', () => {
    expect(returnPathOf('/sign-in?redirect=%2Faccounts')).toBe('/accounts')
    expect(returnPathOf('/sign-in?redirect=%2Fsign-in%3Fredirect%3D%252Faccounts')).toBe(
      '/accounts'
    )
    expect(
      returnPathOf(
        '/sign-in?redirect=%2Fsign-in%3Fredirect%3D%252Fsign-in%253Fredirect%253D%25252Faccounts'
      )
    ).toBe('/accounts')
  })

  it('falls back to the root when there is nothing to unwrap', () => {
    expect(returnPathOf('/sign-in')).toBe('/')
    expect(returnPathOf('//evil.example')).toBe('/')
    expect(returnPathOf('/sign-in?redirect=https%3A%2F%2Fevil.example')).toBe('/')
  })

  // `new URL` normalises before the result is returned, so a path that looks
  // harmless on the way in can come back protocol-relative.
  it('refuses a path that normalises back to protocol-relative', () => {
    expect(returnPathOf('/a/..//evil.example')).toBe('/')
    expect(returnPathOf('/x/%2E%2E//evil.example')).toBe('/')
  })
})
