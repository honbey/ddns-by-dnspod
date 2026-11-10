/** The three preferences the head bootstrap and the React provider agree on. */
export type Theme = 'dark' | 'light' | 'system'

export const DEFAULT_THEME: Theme = 'system'
export const THEME_STORAGE_KEY = 'vite-ui-theme'

/**
 * A stored value is untrusted: another tab on an older build, a hand-edited
 * localStorage or a cleared key can leave anything there, including an empty
 * string. Both readers must fall back the same way, or the first frame and the
 * mounted page disagree. A value outside the enum would otherwise become a CSS
 * class with no rules behind it, and classList.add('') throws.
 *
 * Keep this whitelist in step with the literal check in public/theme-init.js,
 * which cannot import anything because the CSP pins scripts to 'self'.
 */
export function parseTheme(value: unknown, fallback: Theme = DEFAULT_THEME): Theme {
  return value === 'dark' || value === 'light' || value === 'system' ? value : fallback
}
