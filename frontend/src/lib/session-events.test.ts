import { beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * The cross-tab sign-out channel.
 *
 * A cross-tab channel can break without anyone noticing, so the contract is
 * asserted: what a sender puts on the wire, that only a remote sign-out reaches
 * a listener, and that unsubscribing stops delivery.
 *
 * BroadcastChannel is replaced per test because the module caches its channel
 * in module scope - a fresh import is what gives each case its own bus.
 */
class FakeChannel {
  static instances: FakeChannel[] = []

  readonly name: string
  readonly posted: unknown[] = []
  private listeners = new Set<(event: MessageEvent) => void>()

  constructor(name: string) {
    this.name = name
    FakeChannel.instances.push(this)
  }

  postMessage(data: unknown): void {
    this.posted.push(data)
  }

  addEventListener(_type: string, handler: (event: MessageEvent) => void): void {
    this.listeners.add(handler)
  }

  removeEventListener(_type: string, handler: (event: MessageEvent) => void): void {
    this.listeners.delete(handler)
  }

  /** What another tab does: the message arrives at every listener. */
  receive(data: unknown): void {
    for (const handler of [...this.listeners]) handler({ data } as MessageEvent)
  }

  get listenerCount(): number {
    return this.listeners.size
  }
}

async function freshModule() {
  vi.resetModules()
  return import('./session-events')
}

beforeEach(() => {
  FakeChannel.instances = []
  vi.stubGlobal('BroadcastChannel', FakeChannel)
})

describe('broadcastSignOut', () => {
  it('puts one sign-out on the channel', async () => {
    const { broadcastSignOut } = await freshModule()

    broadcastSignOut()

    expect(FakeChannel.instances).toHaveLength(1)
    expect(FakeChannel.instances[0].posted).toEqual(['sign-out'])
    expect(FakeChannel.instances[0].name).toBe('ddns-session')
  })

  it('says nothing when no channel exists', async () => {
    vi.stubGlobal('BroadcastChannel', undefined)
    const { broadcastSignOut, onRemoteSignOut } = await freshModule()

    expect(() => broadcastSignOut()).not.toThrow()
    // A tab without a channel degrades to "other tabs will notice on their
    // next 401" rather than throwing on the way out.
    expect(() => onRemoteSignOut(() => {})).not.toThrow()
  })
})

describe('onRemoteSignOut', () => {
  it('runs the listener for a remote sign-out', async () => {
    const { onRemoteSignOut } = await freshModule()
    const listener = vi.fn()

    onRemoteSignOut(listener)
    FakeChannel.instances[0].receive('sign-out')

    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('ignores anything that is not a sign-out', async () => {
    const { onRemoteSignOut } = await freshModule()
    const listener = vi.fn()

    onRemoteSignOut(listener)
    FakeChannel.instances[0].receive('something-else')
    FakeChannel.instances[0].receive(undefined)

    expect(listener).not.toHaveBeenCalled()
  })

  it('stops delivering once unsubscribed', async () => {
    const { onRemoteSignOut } = await freshModule()
    const listener = vi.fn()

    const unsubscribe = onRemoteSignOut(listener)
    unsubscribe()
    FakeChannel.instances[0].receive('sign-out')

    expect(listener).not.toHaveBeenCalled()
    expect(FakeChannel.instances[0].listenerCount).toBe(0)
  })

  it('does not echo a broadcast back at its own listener', async () => {
    const { broadcastSignOut, onRemoteSignOut } = await freshModule()
    const listener = vi.fn()

    onRemoteSignOut(listener)
    broadcastSignOut()

    // The platform never delivers a message to its own poster, so the only
    // way this could fire is the module calling the listener itself - which is
    // exactly what would re-enter the teardown on this tab's own sign-out.
    expect(listener).not.toHaveBeenCalled()
    expect(FakeChannel.instances[0].posted).toEqual(['sign-out'])
  })
})
