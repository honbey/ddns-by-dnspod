/**
 * A one-shot gate for work that must not run twice at the same time.
 *
 * The session-expiry path uses this to keep a batch of stale requests from
 * toasting and navigating once each. The first caller runs the recovery and
 * every later one is turned away until release() is called - or until a
 * successful login reloads the module and with it this latch.
 */
export function createLatch() {
  let held = false

  return {
    /** True only for the caller that acquired it. */
    tryAcquire(): boolean {
      if (held) return false
      held = true
      return true
    },
    release(): void {
      held = false
    },
  }
}
