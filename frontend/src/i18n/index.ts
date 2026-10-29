/**
 * i18n setup.
 *
 * Two locales are bundled rather than fetched: together they are a few KB, and
 * a network request for them would only add a failure mode to the first paint.
 * Move to i18next-http-backend if this grows past a handful of locales.
 *
 * Chinese is the source dictionary - the interface copy is written in it - and
 * every other locale is typed against its shape, so a missing or misspelled
 * key is a compile error rather than a string that falls back to its own name
 * at runtime.
 */
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { readStored, writeStored } from '@/lib/storage'
import { en } from './locales/en'
import { zh } from './locales/zh'

/** Key under which the choice is remembered; absent means "follow the browser". */
export const LOCALE_KEY = 'app_locale'

export const LOCALES = ['zh', 'en'] as const
export type Locale = (typeof LOCALES)[number]

export const LOCALE_LABELS: Record<Locale, string> = {
  en: 'English',
  zh: '中文',
}

/**
 * Dotted paths of the source dictionary, e.g. `'nav.tasks'`. Slightly stricter
 * than the i18next key type, which also accepts array indexes.
 */
type Leaves<T> = T extends string
  ? never
  : {
      [K in keyof T & string]: T[K] extends string ? K : `${K}.${Leaves<T[K]>}`
    }[keyof T & string]

export type MessageKey = Leaves<typeof zh>

const DEFAULT_LOCALE: Locale = 'zh'

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value)
}

function storedLocale(): Locale | null {
  const stored = readStored(LOCALE_KEY)
  return isLocale(stored) ? stored : null
}

/**
 * Falls back to Chinese for anything that is not English: the interface copy is
 * written in it, so a visitor whose browser asks for a third language is better
 * served by the real thing than by a half-translated mixture.
 */
function browserLocale(): Locale {
  if (typeof navigator === 'undefined') return DEFAULT_LOCALE
  // `languages[0]` is the preference the user actually ordered; `language` is
  // the same value only as long as the browser agrees with itself.
  const [preferred] = navigator.languages ?? []
  const tag = preferred || navigator.language || ''
  return tag.toLowerCase().startsWith('en') ? 'en' : DEFAULT_LOCALE
}

function syncDocumentLang(locale: string): void {
  if (typeof document === 'undefined') return
  document.documentElement.lang = locale === 'zh' ? 'zh-CN' : 'en'
}

const initialLocale = storedLocale() ?? browserLocale()

/** Resolved once the dictionaries are in; the app awaits it before rendering. */
export const i18nReady = i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    zh: { translation: zh },
  },
  lng: initialLocale,
  fallbackLng: DEFAULT_LOCALE,
  supportedLngs: LOCALES,
  interpolation: {
    // `{name}` rather than i18next's default `{{name}}`: the dictionaries read
    // like the copy they hold. React escapes text on render, so no second
    // escaping is needed here.
    prefix: '{',
    suffix: '}',
    escapeValue: false,
  },
  returnNull: false,
  // Nothing this app does involves locize; the support banner is noise in the
  // console of a shipped product.
  showSupportNotice: false,
})

i18n.on('languageChanged', syncDocumentLang)
syncDocumentLang(initialLocale)

/** Switches the interface language and remembers the choice. */
export function setLocale(locale: Locale): void {
  // Stored once the switch has happened, not before: a change that throws
  // would otherwise leave the next visit starting in a language this one never
  // managed to show.
  void i18n.changeLanguage(locale).then(() => writeStored(LOCALE_KEY, locale))
}

/** The active locale, narrowed from i18next's looser `language` string. */
export function currentLocale(): Locale {
  return isLocale(i18n.language) ? i18n.language : DEFAULT_LOCALE
}

/**
 * The BCP-47 tag for Intl formatting. The dictionaries use the short `zh`;
 * `Intl` reads both, but the document language and the formatter should agree
 * on one tag.
 */
export function intlLocale(): string {
  return currentLocale() === 'zh' ? 'zh-CN' : 'en'
}

export { i18n }
