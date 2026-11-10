import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
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

/**
 * Resolved from this file, so the cwd vitest was started from cannot matter.
 * A plain fileURLToPath rather than new URL(..., import.meta.url): Vite reads
 * the latter as an asset import and tries to bundle the backend directory.
 */
const backend = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'backend')

function backendFile(name: string): string {
  return readFileSync(join(backend, name), 'utf8')
}

/** The maxItems inside BatchTaskInput, not one belonging to another schema. */
function documentedCap(text: string): number {
  const lines = text.split('\n')
  const start = lines.findIndex((line) => line.trim() === 'BatchTaskInput:')
  if (start < 0) throw new Error('BatchTaskInput is missing from the contract')
  for (let i = start + 1; i < lines.length; i += 1) {
    if (/^ {4}\w+:$/.test(lines[i])) break
    const match = lines[i].match(/^\s+maxItems:\s*(\d+)\s*$/)
    if (match) return Number(match[1])
  }
  throw new Error('BatchTaskInput.sub_domains.maxItems is missing from the contract')
}

// The cap lives in three places because three layers need it: the contract
// documents it, the server enforces it and the UI names it before a request.
// openapi-typescript emits shapes but not numeric bounds, so no compiler can
// notice one of them drifting; this reads the other two as text instead.
describe('the batch cap in the contract and the server', () => {
  it('is the same number as the UI constant', () => {
    expect(documentedCap(backendFile('openapi.yml'))).toBe(MAX_BATCH_ITEMS)
    const server = backendFile('internal/api/handler_record.go').match(
      /const maxBatchItems = (\d+)/
    )
    expect(server).not.toBeNull()
    expect(Number(server?.[1])).toBe(MAX_BATCH_ITEMS)
  })
})
