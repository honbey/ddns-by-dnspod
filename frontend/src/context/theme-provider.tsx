import { createContext, useContext, useEffect, useLayoutEffect, useState } from 'react'
import { readStored, removeStored, subscribeStored, writeStored } from '@/lib/storage'

type Theme = 'dark' | 'light' | 'system'
type ResolvedTheme = Exclude<Theme, 'system'>

const DEFAULT_THEME = 'system'
const THEME_STORAGE_KEY = 'vite-ui-theme'
/** How long the colour change is allowed to take; the CSS below matches it. */
const THEME_TRANSITION_MS = 200

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeProviderState = {
  defaultTheme: Theme
  resolvedTheme: ResolvedTheme
  theme: Theme
  setTheme: (theme: Theme) => void
  resetTheme: () => void
}

const initialState: ThemeProviderState = {
  defaultTheme: DEFAULT_THEME,
  resolvedTheme: 'light',
  theme: DEFAULT_THEME,
  setTheme: () => null,
  resetTheme: () => null,
}

const ThemeContext = createContext<ThemeProviderState>(initialState)

function systemTheme(): ResolvedTheme {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function ThemeProvider({
  children,
  defaultTheme = DEFAULT_THEME,
  storageKey = THEME_STORAGE_KEY,
  ...props
}: ThemeProviderProps) {
  const [theme, _setTheme] = useState<Theme>(
    () => (readStored(storageKey) as Theme | null) || defaultTheme
  )

  // The colour on screen is state, not a value memoised off `theme`: with the
  // preference set to "system" it has to change when the OS does, so the
  // switch, the theme-color meta and anything else reading it stay in step
  // with the class applied to <html>.
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() =>
    theme === 'system' ? systemTheme() : theme
  )

  useLayoutEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const sync = () =>
      setResolvedTheme(theme === 'system' ? (mediaQuery.matches ? 'dark' : 'light') : theme)

    sync()
    mediaQuery.addEventListener('change', sync)
    return () => mediaQuery.removeEventListener('change', sync)
  }, [theme])

  useLayoutEffect(() => {
    const root = window.document.documentElement
    root.classList.remove('light', 'dark')
    root.classList.add(resolvedTheme)
  }, [resolvedTheme])

  // Every surface takes its colour from the same tokens, but they do not all
  // repaint in the same frame, and the ones that lag read as a flicker. The
  // transition is switched on for the length of the change and no longer, so
  // a dialog opening a second later is not slowed by it.
  useLayoutEffect(() => {
    const root = window.document.documentElement
    root.classList.add('theme-transition')
    const timer = window.setTimeout(
      () => root.classList.remove('theme-transition'),
      THEME_TRANSITION_MS
    )
    return () => window.clearTimeout(timer)
  }, [resolvedTheme])

  // A theme set in another tab applies here too, which is the point of
  // keeping it somewhere every tab can see.
  useEffect(
    () =>
      subscribeStored(storageKey, (value) => _setTheme((value as Theme | null) ?? defaultTheme)),
    [storageKey, defaultTheme]
  )

  const setTheme = (theme: Theme) => {
    writeStored(storageKey, theme)
    _setTheme(theme)
  }

  const resetTheme = () => {
    removeStored(storageKey)
    _setTheme(DEFAULT_THEME)
  }

  const contextValue = {
    defaultTheme,
    resolvedTheme,
    resetTheme,
    theme,
    setTheme,
  }

  return (
    <ThemeContext value={contextValue} {...props}>
      {children}
    </ThemeContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useTheme = () => {
  const context = useContext(ThemeContext)

  if (!context) throw new Error('useTheme must be used within a ThemeProvider')

  return context
}
