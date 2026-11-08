import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { ApiError } from '@/lib/http'
import { clearMissingAccountNotice, createQueryClient } from '@/lib/query-client'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
// The dictionary is loaded elsewhere; assertions only need the message.
vi.mock('@/lib/describe-error', () => ({
  describeError: (error: unknown) => (error as Error).message,
}))

/** What the server answers when no credentials are stored yet. */
function missingAccount(): ApiError {
  return new ApiError('no Tencent Cloud account is configured yet, add one first', 400, {
    code: 'no_account',
  })
}

function failingQuery(client: ReturnType<typeof createQueryClient>, key: string, error: Error) {
  return client.fetchQuery({
    queryKey: [key],
    queryFn: () => Promise.reject(error),
    retry: false,
  })
}

describe('the missing account notice', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.sessionStorage.clear()
    vi.clearAllMocks()
  })

  it('is shown once even when several queries fail the same way', async () => {
    const client = createQueryClient(() => {})

    await expect(failingQuery(client, 'domains', missingAccount())).rejects.toThrow()
    await expect(failingQuery(client, 'records', missingAccount())).rejects.toThrow()

    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('survives a reload, which rebuilds the client', async () => {
    const first = createQueryClient(() => {})
    await expect(failingQuery(first, 'domains', missingAccount())).rejects.toThrow()

    const reloaded = createQueryClient(() => {})
    await expect(failingQuery(reloaded, 'domains', missingAccount())).rejects.toThrow()

    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('speaks again once the notice is cleared', async () => {
    const client = createQueryClient(() => {})
    await expect(failingQuery(client, 'domains', missingAccount())).rejects.toThrow()

    // Saving an account fixes the condition, so the warning is armed again.
    clearMissingAccountNotice()

    await expect(failingQuery(client, 'domains-after-save', missingAccount())).rejects.toThrow()

    expect(toast.error).toHaveBeenCalledTimes(2)
  })

  it('keys on the code, not on the sentence a 400 happens to carry', async () => {
    const client = createQueryClient(() => {})

    // Same words, no code: every occurrence is worth a toast.
    const unlabelled = new ApiError(
      'no Tencent Cloud account is configured yet, add one first',
      400
    )
    await expect(failingQuery(client, 'one', unlabelled)).rejects.toThrow()
    await expect(failingQuery(client, 'two', unlabelled)).rejects.toThrow()

    expect(toast.error).toHaveBeenCalledTimes(2)
  })

  it('does not silence other failures', async () => {
    const client = createQueryClient(() => {})

    await expect(failingQuery(client, 'one', new ApiError('something else', 400))).rejects.toThrow()
    await expect(failingQuery(client, 'two', new ApiError('something else', 400))).rejects.toThrow()

    expect(toast.error).toHaveBeenCalledTimes(2)
  })
})
