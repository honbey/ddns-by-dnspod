import { createContext, useContext, useEffect, useState, useMemo } from 'react'
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

export function ThemeProvider({
  children,
  defaultTheme = DEFAULT_THEME,
  storageKey = THEME_STORAGE_KEY,
  ...props
}: ThemeProviderProps) {
  const [theme, _setTheme] = useState<Theme>(
    () => (readStored(storageKey) as Theme | null) || defaultTheme
  )

  // Optimized: Memoize the resolved theme calculation to prevent unnecessary re-computations
  const resolvedTheme = useMemo((): ResolvedTheme => {
    if (theme === 'system') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    }
    return theme as ResolvedTheme
  }, [theme])

  useEffect(() => {
    const root = window.document.documentElement
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')

    const applyTheme = (currentResolvedTheme: ResolvedTheme) => {
      root.classList.remove('light', 'dark') // Remove existing theme classes
      root.classList.add(currentResolvedTheme) // Add the new theme class
    }

    const handleChange = () => {
      if (theme === 'system') {
        const systemTheme = mediaQuery.matches ? 'dark' : 'light'
        applyTheme(systemTheme)
      }
    }

    applyTheme(resolvedTheme)

    mediaQuery.addEventListener('change', handleChange)

    return () => mediaQuery.removeEventListener('change', handleChange)
  }, [theme, resolvedTheme])

  // Every surface takes its colour from the same tokens, but they do not all
  // repaint in the same frame, and the ones that lag read as a flicker. The
  // transition is switched on for the length of the change and no longer, so
  // a dialog opening a second later is not slowed by it.
  useEffect(() => {
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
