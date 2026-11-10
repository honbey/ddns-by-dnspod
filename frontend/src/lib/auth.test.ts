import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const manager = vi.hoisted(() => ({
  getUser: vi.fn(),
  removeUser: vi.fn(),
  metadataService: {
    getEndSessionEndpoint: vi.fn(),
    getRevocationEndpoint: vi.fn(),
  },
}))

vi.mock('@/lib/oidc', () => ({
  getUserManager: async () => manager,
}))

vi.mock('@/lib/session-events', () => ({
  broadcastSignOut: vi.fn(),
}))

import { displayName, signOut } from './auth'
import { useAuthStore } from '@/stores/auth-store'

describe('sign-out', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.reset()
    manager.getUser.mockResolvedValue({ expired: false, refresh_token: 'refresh' })
    manager.removeUser.mockResolvedValue(undefined)
    manager.metadataService.getRevocationEndpoint.mockResolvedValue(undefined)
  })

  // A failed assertion used to leave fake timers and the location spy in
  // place, so the next case ran against the wrong clock. Vitest has no
  // restoreMocks configured, so the cleanup belongs here.
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('clears the local session when provider metadata hangs', async () => {
    vi.useFakeTimers()
    const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {})
    manager.metadataService.getEndSessionEndpoint.mockReturnValue(new Promise(() => {}))

    const pending = signOut('provider')
    await vi.advanceTimersByTimeAsync(5_000)
    await expect(pending).resolves.toBeUndefined()

    expect(useAuthStore.getState().auth.user).toBeNull()
    expect(manager.removeUser).toHaveBeenCalled()
    expect(assign).toHaveBeenCalledWith('/')
  })
})

describe('displayName', () => {
  const userWith = (profile: Record<string, unknown>) =>
    ({ profile }) as unknown as Parameters<typeof displayName>[0]

  it('walks past an empty claim to the next one', () => {
    expect(
      displayName(userWith({ name: '', preferred_username: '', email: 'a@example.com', sub: 'id' }))
    ).toBe('a@example.com')
  })

  it('falls back to User when every claim is empty', () => {
    expect(displayName(userWith({ name: '', preferred_username: '', email: '', sub: '' }))).toBe(
      'User'
    )
  })

  it('keeps the same fallback for a signed-out session', () => {
    expect(displayName(null)).toBe('User')
  })
})
