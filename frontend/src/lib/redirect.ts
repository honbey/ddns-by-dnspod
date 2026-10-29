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
