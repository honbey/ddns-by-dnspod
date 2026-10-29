/**
 * How many records one batch call may carry.
 *
 * The API refuses a longer list outright - its own cap is maxBatchItems in the
 * backend - and every batch path uses the same number. Kept here so a dialog or
 * a table can name the limit before the call, instead of each one learning it
 * from a 400 after a confirmation that could not have worked.
 */
export const MAX_BATCH_ITEMS = 100

/** Whether this many records fit in one batch call. */
export function fitsOneBatch(count: number): boolean {
  return count <= MAX_BATCH_ITEMS
}
