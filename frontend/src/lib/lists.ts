/**
 * Lists that do not fit one call.
 *
 * The API caps a page at 200 rows and answers one page at a time, while a bulk
 * action sends one request per row. Both are handled here so a page does not
 * repeat the arithmetic, and so one click cannot turn into a burst that the
 * provider answers by refusing the later calls.
 */

/** The largest page the API serves; its own cap is maxPageSize in paging.go. */
export const API_PAGE_SIZE = 200

/**
 * How many pages one view reads before it stops.
 *
 * Thousands of records must not turn opening a page into an unbounded walk;
 * past this the view says how much of the list it is showing instead.
 */
export const MAX_LIST_PAGES = 15

/**
 * A list read page by page.
 *
 * `total` is the API's own count, so it is larger than `items` exactly when the
 * ceiling stopped the walk short.
 */
export interface CollectedList<T> {
  items: T[]
  total: number
}

/**
 * Reads pages until the whole list is in hand, or until the page ceiling.
 *
 * `total` comes from the answers, so a caller can tell a complete list from a
 * truncated one.
 */
export async function collectPages<T>(
  fetchPage: (page: number) => Promise<CollectedList<T>>
): Promise<CollectedList<T>> {
  const items: T[] = []
  let total = 0

  for (let page = 1; page <= MAX_LIST_PAGES; page++) {
    const chunk = await fetchPage(page)
    items.push(...chunk.items)
    total = chunk.total
    // A short page is the end of the list, and so is having read everything the
    // API says matches; either way one more call would only repeat the answer.
    if (chunk.items.length < API_PAGE_SIZE || items.length >= total) break
  }

  return { items, total }
}

/**
 * Runs one job per item, at most `limit` at a time.
 *
 * "Run all" used to fire every request in the same instant: two hundred tasks
 * meant two hundred calls to DNSPod at once, which is how a click in the
 * console runs into the provider's rate limit.
 *
 * A job has to own its failures: report them the way the single item path does
 * rather than rejecting, so one bad item neither stops the pool nor loses the
 * rest.
 */
export async function mapBounded<T>(
  items: readonly T[],
  limit: number,
  job: (item: T) => Promise<unknown>
): Promise<void> {
  let next = 0
  const workers = Array.from({ length: Math.min(Math.max(limit, 1), items.length) }, async () => {
    while (next < items.length) {
      const item = items[next]
      next += 1
      await job(item)
    }
  })
  await Promise.all(workers)
}
