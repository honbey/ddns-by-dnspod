import { describe, expect, it } from 'vitest'
import { fitsOneBatch, MAX_BATCH_ITEMS } from './limits'

// The cap is a contract with the API, which refuses a longer list outright.
// Pinned at the boundary, and pinned to the number itself: the backend enforces
// the same 100, so moving one side has to be a deliberate act on both.
describe('fitsOneBatch', () => {
  it('accepts a batch at the cap and refuses one over it', () => {
    expect(fitsOneBatch(MAX_BATCH_ITEMS)).toBe(true)
    expect(fitsOneBatch(MAX_BATCH_ITEMS + 1)).toBe(false)
  })

  it('is the number the API enforces', () => {
    expect(MAX_BATCH_ITEMS).toBe(100)
  })

  it('accepts an empty selection', () => {
    expect(fitsOneBatch(0)).toBe(true)
  })
})
