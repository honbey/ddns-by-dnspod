/**
 * How many records one batch call may carry.
 *
 * The contract states this as BatchTaskInput.sub_domains.maxItems and the
 * server as maxBatchItems. Generated TypeScript types cannot carry a numeric
 * bound, so the UI still needs a constant to name the limit before a call
 * instead of learning it from a 400. limits.test.ts ties the three together.
 */
export const MAX_BATCH_ITEMS = 100

/** Whether this many records fit in one batch call. */
export function fitsOneBatch(count: number): boolean {
  return count <= MAX_BATCH_ITEMS
}
