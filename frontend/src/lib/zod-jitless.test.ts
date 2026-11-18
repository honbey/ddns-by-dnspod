import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import './zod-jitless'

describe('zod-jitless', () => {
  it('leaves the compiler off, so no parse probes the CSP', () => {
    expect(z.config().jitless).toBe(true)
  })

  it('still parses, on the path a strict CSP would get anyway', () => {
    const schema = z.object({ host: z.string().min(1) })
    expect(schema.safeParse({ host: 'home' }).success).toBe(true)
    expect(schema.safeParse({ host: '' }).success).toBe(false)
  })
})
