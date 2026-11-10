import { fitsOneBatch } from '@/lib/limits'

/**
 * Splits the subdomain field into hosts, on spaces or English commas.
 *
 * An empty field still means the apex, as it did when the field held one host:
 * the server turns an empty sub domain into "@", and the batch endpoint takes
 * the same shape for every entry.
 */
export function parseSubDomains(raw: string): string[] {
  const parts = raw
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter(Boolean)
  if (parts.length === 0) return ['@']
  // One host typed twice is one task, not a duplicate the server has to skip.
  return [...new Set(parts)]
}

/** Whether the field fits the batch limit before a request is made. */
export function fitsSubDomainBatch(raw: string): boolean {
  return fitsOneBatch(parseSubDomains(raw).length)
}
