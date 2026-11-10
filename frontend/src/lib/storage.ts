/**
 * Where a preference is kept: localStorage while it can be written to, then
 * sessionStorage, then a Map for this page load.
 *
 * These are preferences - theme, layout, font, direction, whether the sidebar
 * is open - and nothing on the server reads them, so a cookie buys nothing
 * here except being sent on every request. What matters instead is that a
 * browser is allowed to refuse storage: private windows, blocked origins and
 * full quotas all throw on write. The chain is tried in order and the first
 * one that accepts a write is the one used from then on, so a preference is
 * remembered for as long as this browser lets it be, and a refusal costs a
 * setting rather than the page.
 *
 * Credentials are deliberately not stored here: the OIDC token lives in
 * sessionStorage because it is meant to die with the tab, and a fallback that
 * outlives the tab would be the wrong direction for it.
 */

type KeyValueStore = {
  readonly name: string
  get: (key: string) => string | null
  set: (key: string, value: string) => void
  remove: (key: string) => void
}

/** Written and removed once: the only way to know a store takes writes. */
const PROBE_KEY = '__ddns_probe__'

function storageOf(name: string): Storage | undefined {
  if (typeof window === 'undefined') return undefined
  try {
    return window[name === 'localStorage' ? 'localStorage' : 'sessionStorage']
  } catch {
    // Touching the property is itself refused in some privacy modes.
    return undefined
  }
}

function webStore(name: 'localStorage' | 'sessionStorage'): KeyValueStore | null {
  const store = storageOf(name)
  if (!store) return null

  try {
    store.setItem(PROBE_KEY, '1')
    store.removeItem(PROBE_KEY)
  } catch {
    return null
  }

  return {
    name,
    get: (key) => store.getItem(key),
    set: (key, value) => store.setItem(key, value),
    remove: (key) => store.removeItem(key),
  }
}

/** The last resort: this page load only, and never shared with another tab. */
function memoryStore(): KeyValueStore {
  const values = new Map<string, string>()
  return {
    name: 'memory',
    get: (key) => values.get(key) ?? null,
    set: (key, value) => {
      values.set(key, value)
    },
    remove: (key) => {
      values.delete(key)
    },
  }
}

const stores: KeyValueStore[] = [
  webStore('localStorage'),
  webStore('sessionStorage'),
  memoryStore(),
].filter((store): store is KeyValueStore => store !== null)

let active: KeyValueStore = stores[0] ?? memoryStore()

/** A stored preference, or `null` when it was never set here. */
export function readStored(key: string): string | null {
  try {
    return active.get(key)
  } catch {
    return null
  }
}

export function writeStored(key: string, value: string): void {
  // A store can pass the probe and still refuse the real write - the quota
  // filled between the two, say - so the rest of the chain is tried, and the
  // one that accepts becomes the one used from here on.
  const from = Math.max(stores.indexOf(active), 0)
  for (const store of stores.slice(from)) {
    try {
      store.set(key, value)
      active = store
      return
    } catch {
      continue
    }
  }
}

export function removeStored(key: string): void {
  const from = Math.max(stores.indexOf(active), 0)
  for (const store of stores.slice(from)) {
    try {
      store.remove(key)
      active = store
      return
    } catch {
      continue
    }
  }
}

/**
 * Follows a preference as another tab changes it. `storage` fires only in the
 * *other* tabs of the same origin, and only for localStorage, so the two
 * fallbacks simply never notify - a value changed elsewhere reaches them on
 * the next load, which is all a per-tab store can promise.
 */
export function subscribeStored(key: string, onChange: (value: string | null) => void): () => void {
  if (typeof window === 'undefined') return () => {}

  const handler = (event: StorageEvent) => {
    // Only localStorage emits these and only localStorage is shared between
    // tabs. When this tab fell back to sessionStorage or memory, another tab's
    // localStorage event cannot change a preference kept here.
    if (active.name !== 'localStorage') return
    // `null` is `clear()`: every key went, including this one.
    if (event.key !== null && event.key !== key) return
    onChange(event.key === null ? null : event.newValue)
  }

  window.addEventListener('storage', handler)
  return () => window.removeEventListener('storage', handler)
}
