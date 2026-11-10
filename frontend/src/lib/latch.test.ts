import { describe, expect, it } from 'vitest'
import { createLatch } from './latch'

describe('createLatch', () => {
  it('lets exactly one caller through while held', () => {
    const latch = createLatch()

    expect(latch.tryAcquire()).toBe(true)
    expect(latch.tryAcquire()).toBe(false)
    expect(latch.tryAcquire()).toBe(false)
  })

  it('lets the next caller through after release', () => {
    const latch = createLatch()

    expect(latch.tryAcquire()).toBe(true)
    latch.release()
    expect(latch.tryAcquire()).toBe(true)
  })

  it('treats a release while disengaged as a no-op', () => {
    const latch = createLatch()

    latch.release()
    expect(latch.tryAcquire()).toBe(true)
    latch.release()
    latch.release()
    expect(latch.tryAcquire()).toBe(true)
  })
})
