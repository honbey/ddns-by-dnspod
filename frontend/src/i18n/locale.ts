/**
 * How a language is chosen, kept free of i18next so the decision can be tested
 * without starting the app.
 *
 * The stored preference and the language on screen are different things: a
 * visitor following the system has no preference stored, yet still sees a real
 * language. Keeping that resolution here is what lets the UI offer "follow the
 * system" as an option without writing the resolved value back to storage.
 */
export const LOCALES = ['zh', 'en'] as const
export type Locale = (typeof LOCALES)[number]
export type LocalePreference = Locale | 'system'

export const DEFAULT_LOCALE: Locale = 'zh'

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value)
}

/**
 * An explicit choice wins. Otherwise the first browser language decides, and
 * anything that is not English falls back to Chinese: the interface copy is
 * written in it, so a half-translated mixture would be worse than the real
 * thing. An absent or unrecognised stored value means follow the system.
 */
export function resolveLocale(stored: string | null, languages: readonly string[]): Locale {
  if (isLocale(stored)) return stored
  const tag = languages[0] ?? ''
  return tag.toLowerCase().startsWith('en') ? 'en' : DEFAULT_LOCALE
}

/** The preference carried by a storage event, not read from this tab's store. */
export function preferenceFromStorage(value: string | null): LocalePreference {
  return isLocale(value) ? value : 'system'
}

/**
 * Guards async preference changes. i18next serialises `changeLanguage` calls,
 * so the last request is the only one whose callback may persist a choice;
 * an older one would store a language that is no longer on screen.
 */
export function createEpochGate() {
  let current = 0

  return {
    next(): number {
      current += 1
      return current
    },
    isCurrent(epoch: number): boolean {
      return epoch === current
    },
  }
}
