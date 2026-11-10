/*
 * Runs in <head>, before any stylesheet or module. The CSP pins scripts to
 * 'self', so this has to be a real file rather than an inline script. React's
 * ThemeProvider reaches the same result later; doing it here means a dark
 * preference never paints a white first frame while the app boots.
 *
 * Best effort: lib/storage.ts chooses its active store with a write probe at
 * module load, and can move to a later store when a real write is refused.
 * A static script cannot see that in-memory choice, so it repeats the probe
 * here. A session that fell back at runtime can still disagree for one frame;
 * ThemeProvider's layout effect settles it on mount.
 */
;(function () {
  // Must stay in step with THEME_STORAGE_KEY and the parseTheme whitelist in
  // src/lib/theme.ts. A classic script under this CSP cannot import them.
  var key = 'vite-ui-theme'
  var probe = '__ddns_probe__'

  // Same order and same "writable, not merely readable" rule as
  // lib/storage.ts: each store gets its own try, so a refusal in one cannot
  // stop the next from answering. readStored only consults the active store
  // and does not look further when it has no value, so neither do we.
  function readPreference() {
    for (var i = 0; i < 2; i += 1) {
      var store
      try {
        store = i === 0 ? window.localStorage : window.sessionStorage
        store.setItem(probe, '1')
        store.removeItem(probe)
      } catch {
        // Refused, or touching the property itself threw. Try the next.
        continue
      }
      // readStored only consults the active store and swallows a failed read,
      // so a failure here means "no preference", not "try the next store".
      try {
        return store.getItem(key)
      } catch {
        return null
      }
    }
    return null
  }

  // Equivalent to parseTheme(value, DEFAULT_THEME) in src/lib/theme.ts.
  var stored = readPreference()
  var preference =
    stored === 'dark' || stored === 'light' || stored === 'system' ? stored : 'system'
  var systemIsDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  var dark = preference === 'dark' || (preference === 'system' && systemIsDark)

  var root = document.documentElement
  root.classList.remove('light', 'dark')
  root.classList.add(dark ? 'dark' : 'light')

  // Match --background in theme.css, not a colour of its own: the browser
  // chrome should read as a continuation of the page on the first frame too.
  var meta = document.querySelector("meta[name='theme-color']")
  if (meta) meta.setAttribute('content', dark ? '#000000' : '#ffffff')
})()
