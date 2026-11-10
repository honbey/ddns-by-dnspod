/**
 * The post-login destination, or '/' when the value cannot be one.
 *
 * The value arrives in the OIDC `state` and back as a query parameter, so a
 * crafted link can reach it. Control characters are stripped before the checks
 * because browsers drop tab and newline while parsing: `/\t/evil.com` would
 * pass a prefix test and then be read as `//evil.com`.
 */
export function safeReturnPath(value: unknown): string {
  if (typeof value !== 'string') return '/'

  // Stripping control characters is what this does: the class is the defence,
  // not an accident.
  // eslint-disable-next-line no-control-regex
  const path = value.replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim()
  if (!path.startsWith('/')) return '/'
  // "//host" is protocol-relative and "/\" is read as "//" by some browsers.
  if (path.startsWith('//') || path.startsWith('/\\')) return '/'
  return path
}

/**
 * The path to return to, with sign-in hops unwrapped.
 *
 * A stale request can fail after the session has already sent the user to
 * /sign-in. The href then names the sign-in page, and its `redirect` parameter
 * holds the real target - so reading it back out is what stops each failure
 * from nesting another copy of the redirect into the next one.
 *
 * Every candidate goes through safeReturnPath, so unwrapping cannot leave this
 * origin, and the depth is bounded in case the parameter points at itself.
 */
export function returnPathOf(href: string): string {
  let candidate = safeReturnPath(href)
  for (let depth = 0; depth < 8; depth += 1) {
    const url = new URL(candidate, window.location.origin)
    if (url.pathname !== '/sign-in') {
      // `new URL` normalises, and normalising can turn a path back into a
      // protocol-relative one, so the result goes through the guard again
      // rather than trusting the input check alone.
      return safeReturnPath(`${url.pathname}${url.search}${url.hash}`)
    }
    const next = url.searchParams.get('redirect')
    if (!next) return '/'
    candidate = safeReturnPath(next)
  }
  return '/'
}
