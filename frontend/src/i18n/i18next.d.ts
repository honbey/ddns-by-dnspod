import type { zh } from './locales/zh'

/**
 * Makes `t()` reject a key that is not in the source dictionary and offers
 * completion for the ones that are.
 */
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: {
      translation: typeof zh
    }
  }
}

export {}
