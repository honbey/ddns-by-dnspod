import { describe, expect, it, vi } from 'vitest'
import { API_PAGE_SIZE, MAX_LIST_PAGES, collectPages, mapBounded } from './lists'

/** A page answering with `count` rows out of `total`. */
function pageOf(count: number, total: number, start = 1) {
  return {
    items: Array.from({ length: count }, (_value, index) => start + index),
    total,
  }
}

describe('collectPages', () => {
  it('stops after a short page', async () => {
    const fetchPage = vi
      .fn()
      .mockResolvedValueOnce(pageOf(API_PAGE_SIZE, 250))
      .mockResolvedValueOnce(pageOf(50, 250, API_PAGE_SIZE + 1))

    const result = await collectPages(fetchPage)

    expect(fetchPage).toHaveBeenCalledTimes(2)
    expect(result.items).toHaveLength(250)
    expect(result.total).toBe(250)
  })

  it('stops as soon as it has read everything the API counted', async () => {
    const fetchPage = vi.fn().mockResolvedValue(pageOf(API_PAGE_SIZE, API_PAGE_SIZE))

    await collectPages(fetchPage)

    expect(fetchPage).toHaveBeenCalledTimes(1)
  })

  it('walks full pages until the ceiling, then reports the shortfall', async () => {
    const fetchPage = vi.fn().mockResolvedValue(pageOf(API_PAGE_SIZE, 100_000))

    const result = await collectPages(fetchPage)

    expect(fetchPage).toHaveBeenCalledTimes(MAX_LIST_PAGES)
    expect(result.items).toHaveLength(API_PAGE_SIZE * MAX_LIST_PAGES)
    expect(result.total).toBe(100_000)
  })

  it('asks for the pages in order', async () => {
    const fetchPage = vi
      .fn()
      .mockResolvedValueOnce(pageOf(API_PAGE_SIZE, 300))
      .mockResolvedValueOnce(pageOf(100, 300, API_PAGE_SIZE + 1))

    await collectPages(fetchPage)

    expect(fetchPage).toHaveBeenNthCalledWith(1, 1)
    expect(fetchPage).toHaveBeenNthCalledWith(2, 2)
  })
})

describe('mapBounded', () => {
  it('never runs more jobs at once than the limit', async () => {
    let inFlight = 0
    let peak = 0

    await mapBounded([1, 2, 3, 4, 5, 6, 7], 3, async () => {
      inFlight += 1
      peak = Math.max(peak, inFlight)
      await new Promise((resolve) => setTimeout(resolve, 1))
      inFlight -= 1
    })

    expect(peak).toBe(3)
  })

  it('runs every item exactly once', async () => {
    const seen: number[] = []

    await mapBounded([1, 2, 3, 4, 5], 2, async (item) => {
      seen.push(item)
    })

    expect(seen.sort((a, b) => a - b)).toEqual([1, 2, 3, 4, 5])
  })

  it('starts every item even when one of them reports a failure', async () => {
    const attempted: number[] = []
    const failed: number[] = []

    await mapBounded([1, 2, 3], 1, async (item) => {
      attempted.push(item)
      // What a job does with its own failure: report it, stay resolved.
      await Promise.reject(new Error('item failed')).catch(() => failed.push(item))
    })

    expect(attempted).toEqual([1, 2, 3])
    expect(failed).toEqual([1, 2, 3])
  })

  it('resolves without running anything for an empty list', async () => {
    const job = vi.fn()

    await mapBounded([], 4, job)

    expect(job).not.toHaveBeenCalled()
  })
})
