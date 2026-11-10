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
import { readStored, removeStored, subscribeStored, writeStored } from '@/lib/storage'
import { en } from './locales/en'
import { zh } from './locales/zh'
import {
  DEFAULT_LOCALE,
  LOCALES,
  createEpochGate,
  isLocale,
  preferenceFromStorage,
  resolveLocale,
  type Locale,
  type LocalePreference,
} from './locale'

export { LOCALES, isLocale, resolveLocale, type Locale, type LocalePreference } from './locale'

/** Key under which the choice is remembered; absent means "follow the browser". */
export const LOCALE_KEY = 'app_locale'

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

function browserLanguages(): readonly string[] {
  if (typeof navigator === 'undefined') return []
  // `languages[0]` is the preference the user actually ordered; `language` is
  // the same value only as long as the browser agrees with itself.
  return navigator.languages?.length ? navigator.languages : [navigator.language]
}

function browserLocale(): Locale {
  return resolveLocale(null, browserLanguages())
}

function syncDocumentLang(locale: string): void {
  if (typeof document === 'undefined') return
  document.documentElement.lang = locale === 'zh' ? 'zh-CN' : 'en'
}

const initialLocale = resolveLocale(readStored(LOCALE_KEY), browserLanguages())

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

/**
 * Preference changes are local to this module: a `storage` event only fires in
 * *other* tabs, so the tab that made the choice needs its own notification.
 */
const preferenceListeners = new Set<() => void>()

function notifyPreferenceListeners(): void {
  for (const listener of preferenceListeners) listener()
}

export function subscribeLocalePreference(listener: () => void): () => void {
  preferenceListeners.add(listener)
  return () => {
    preferenceListeners.delete(listener)
  }
}

/** The stored choice; an absent or unknown value means follow the system. */
export function localePreference(): LocalePreference {
  return preferenceFromStorage(readStored(LOCALE_KEY))
}

const epochGate = createEpochGate()
/** An explicit choice waiting to be persisted; OS changes must not steal it. */
let pendingPersist: LocalePreference | null = null

/**
 * Every source of a language change goes through here. The last request wins:
 * i18next serialises the calls, and only its callback may persist a choice, so
 * a slower one cannot store a language that is no longer on screen.
 */
function requestLocale(preference: LocalePreference, persist: boolean): void {
  const epoch = epochGate.next()
  const target = preference === 'system' ? browserLocale() : preference
  if (persist) pendingPersist = preference

  void i18n.changeLanguage(target).then(
    () => {
      if (!epochGate.isCurrent(epoch)) return
      if (persist) {
        // Stored once the switch has happened, not before: a change that throws
        // would otherwise leave the next visit starting in a language never shown.
        if (preference === 'system') removeStored(LOCALE_KEY)
        else writeStored(LOCALE_KEY, preference)
        pendingPersist = null
      }
      notifyPreferenceListeners()
    },
    () => {
      if (!epochGate.isCurrent(epoch)) return
      // A failed change leaves the old language on screen. The UI still has to
      // re-read the stored preference rather than keep a choice never applied.
      pendingPersist = null
      notifyPreferenceListeners()
    }
  )
}

/**
 * Applies a preference. Following the system clears the stored value instead
 * of writing the resolved language back, or the next visit would start with a
 * choice this visitor never made.
 */
export function setLocalePreference(preference: LocalePreference): void {
  requestLocale(preference, true)
}

if (typeof window !== 'undefined') {
  // The browser language can change while the page is open. Only a visitor
  // following the system should move, and a pending explicit choice is this
  // tab's to finish rather than the OS's to cancel.
  window.addEventListener('languagechange', () => {
    if (pendingPersist !== null || localePreference() !== 'system') return
    requestLocale('system', false)
  })
}

// Another tab may set a language or return to the system default. The event
// value is authoritative: when this tab's active store fell back to
// sessionStorage or memory, reading it here would see null and turn an
// explicit choice back into "system".
subscribeStored(LOCALE_KEY, (value) => {
  pendingPersist = null
  requestLocale(preferenceFromStorage(value), false)
})

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
