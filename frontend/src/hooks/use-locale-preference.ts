import { useSyncExternalStore } from 'react'
import { localePreference, subscribeLocalePreference, type LocalePreference } from '@/i18n'

/**
 * The stored language choice, kept in sync in this tab and across tabs.
 * `useSyncExternalStore` reads the value on every render, so a preference that
 * resolves to the same language still updates the "follow the system" state.
 */
export function useLocalePreference(): LocalePreference {
  return useSyncExternalStore(subscribeLocalePreference, localePreference, () => 'system')
}
