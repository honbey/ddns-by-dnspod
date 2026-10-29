/**
 * Cross-tab session events.
 *
 * The OIDC token lives in sessionStorage, which is per tab: signing out in one
 * leaves the others holding a token that still works. This channel is what
 * makes the *UI* agree across tabs - the local state goes away everywhere the
 * moment one tab signs out. (An already-issued access token stays valid at the
 * issuer until it expires; only a revocation-capable provider ends that, which
 * sign-out already attempts.)
 */
const CHANNEL_NAME = 'ddns-session'

let bus: BroadcastChannel | null | undefined

function channel(): BroadcastChannel | null {
  if (bus !== undefined) return bus
  bus = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel(CHANNEL_NAME)
  return bus
}

/** Tells the other tabs this session is over. Never loops: only senders send. */
export function broadcastSignOut(): void {
  try {
    channel()?.postMessage('sign-out')
  } catch {
    // A closed channel or a browser that changed its mind: the other tabs
    // will notice on their next 401.
  }
}

/** Runs in every *other* tab when one of them signs out. */
export function onRemoteSignOut(listener: () => void): () => void {
  const open = channel()
  if (!open) return () => {}

  const handler = (event: MessageEvent) => {
    if (event.data === 'sign-out') listener()
  }
  open.addEventListener('message', handler)
  return () => open.removeEventListener('message', handler)
}
