import { describe, expect, it, vi } from 'vitest'

/**
 * `subscribeStored` watches `window`'s storage events, which only ever come
 * from localStorage. If this tab had to fall back to sessionStorage or memory,
 * another tab's localStorage event must not rewrite a preference kept here.
 */
describe('subscribeStored', () => {
  it('notifies when localStorage is the active store', async () => {
    vi.resetModules()
    const { subscribeStored } = await import('./storage')

    const seen: Array<string | null> = []
    const unsubscribe = subscribeStored('probe', (value) => seen.push(value))
    window.dispatchEvent(new StorageEvent('storage', { key: 'probe', newValue: 'en' }))

    expect(seen).toEqual(['en'])
    unsubscribe()
  })

  it('ignores localStorage events after a fallback to another store', async () => {
    const descriptor = Object.getOwnPropertyDescriptor(window, 'localStorage')
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      get() {
        throw new Error('storage blocked')
      },
    })

    try {
      vi.resetModules()
      const { subscribeStored, readStored, writeStored } = await import('./storage')

      writeStored('probe', 'zh')
      expect(readStored('probe')).toBe('zh')

      let called = false
      const unsubscribe = subscribeStored('probe', () => {
        called = true
      })
      window.dispatchEvent(new StorageEvent('storage', { key: 'probe', newValue: 'en' }))

      expect(called).toBe(false)
      expect(readStored('probe')).toBe('zh')
      unsubscribe()
    } finally {
      if (descriptor) Object.defineProperty(window, 'localStorage', descriptor)
      else Reflect.deleteProperty(window, 'localStorage')
      vi.resetModules()
    }
  })
})
