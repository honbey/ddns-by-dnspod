import { beforeEach, describe, expect, it, vi } from 'vitest'

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

import { signOut } from './auth'
import { useAuthStore } from '@/stores/auth-store'

describe('sign-out', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.reset()
    manager.getUser.mockResolvedValue({ expired: false, refresh_token: 'refresh' })
    manager.removeUser.mockResolvedValue(undefined)
    manager.metadataService.getRevocationEndpoint.mockResolvedValue(undefined)
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
    assign.mockRestore()
    vi.useRealTimers()
  })
})
