import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { en } from './locales/en'
import { zh } from './locales/zh'

/**
 * The dictionaries are typed, so a missing key is already a compile error.
 * What a type cannot check is the shape *inside* a value: an interpolation the
 * translation forgot to carry over renders its braces as literal text, which
 * would ship unnoticed.
 */
function flatten(source: Record<string, unknown>, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(source)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof value === 'string') {
      out[path] = value
    } else if (value && typeof value === 'object') {
      Object.assign(out, flatten(value as Record<string, unknown>, path))
    }
  }
  return out
}

/** The `{name}` placeholders one string interpolates, in the order they appear. */
function placeholders(text: string): string[] {
  return Array.from(text.matchAll(/\{([^}]+)\}/g), (match) => match[1]).sort()
}

describe('locales', () => {
  const source = flatten(zh as unknown as Record<string, unknown>)
  const translated = flatten(en as unknown as Record<string, unknown>)

  it('holds at least the strings the shell needs', () => {
    expect(Object.keys(source).length).toBeGreaterThan(20)
  })

  it('translates every key of the source', () => {
    const missing = Object.keys(source).filter((key) => !(key in translated))
    expect(missing).toEqual([])
  })

  it('keeps every interpolation the source uses', () => {
    const wrong = Object.entries(source)
      .filter(([key, value]) => {
        const other = translated[key]
        return other !== undefined && placeholders(value).join() !== placeholders(other).join()
      })
      .map(([key]) => key)
    expect(wrong).toEqual([])
  })

  it('translates everything outside the allowlist', () => {
    // A copy that is byte-for-byte identical to the source is either a term no
    // Chinese page would translate or work that was skipped. Spelling those
    // out beats a quota: the list is the decision, and adding to it is a
    // deliberate act rather than bumping a number.
    const ALLOWED = new Set<string>([
      // Examples, not copy: the placeholder shows the shape of what to type.
      'tasks.domainPlaceholder',
      'tasks.subdomainPlaceholder',
      'records.hostPlaceholder',
      'certificates.namePlaceholder',
      'apiKeys.domainPlaceholder',
      // Product terms: they are written this way in both languages.
      'accounts.secretIdLabel',
      'accounts.secretKeyLabel',
      'notifications.serverPlaceholder',
      'notifications.addressPlaceholder',
      'notifications.topicPlaceholder',
      'notifications.tagsPlaceholder',
    ])
    const identical = Object.keys(source)
      .filter((key) => translated[key] === source[key])
      .filter((key) => /[a-z]{4,}/i.test(source[key]))
      .filter((key) => !ALLOWED.has(key))
    expect(identical).toEqual([])
  })
})

/**
 * Keys the code assembles at runtime, which no literal scan can see. Each one
 * is an explicit decision, not a wildcard: it must name a real prefix that the
 * call site builds from a closed set of values.
 */
const DYNAMIC_PREFIXES = ['notifications.priority']

/** i18next resolves these from the base key the call site names. */
const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/

/** Every source file the scan reads. */
function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) {
      // The dictionaries are the data, not a consumer of it.
      if (path.endsWith(join('i18n', 'locales'))) return []
      return sourceFiles(path)
    }
    if (!/\.tsx?$/.test(entry)) return []
    if (path.endsWith('i18n.test.ts')) return []
    return [path]
  })
}

/**
 * Drops comments so a key named in prose is not counted as a reference. `//` is
 * only taken as a comment start after whitespace, which keeps `https://` intact.
 */
function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|\s)\/\/[^\n]*/g, '$1')
}

/**
 * The keys one file refers to: literal `t('a.b')` calls, plus any quoted string
 * that is exactly a dictionary key, because navigation titles and other message
 * keys are passed around as data rather than called.
 *
 * A line may opt out with `i18n-exempt`, for a test that checks the fallback of
 * a key deliberately absent.
 */
function referencedKeys(source: string, known: Set<string>): string[] {
  const found: string[] = []
  for (const raw of stripComments(source).split('\n')) {
    if (raw.includes('i18n-exempt')) continue
    for (const match of raw.matchAll(/\b(?:t|translate)\(\s*['"]([A-Za-z0-9_.]+)['"]/g)) {
      found.push(match[1])
    }
    for (const match of raw.matchAll(/['"]([A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+)['"]/g)) {
      if (known.has(match[1])) found.push(match[1])
    }
  }
  return found
}

describe('dictionary coverage', () => {
  const SRC = join(process.cwd(), 'src')
  const known = new Set(Object.keys(flatten(zh as unknown as Record<string, unknown>)))
  const referenced = new Set<string>()

  for (const file of sourceFiles(SRC)) {
    for (const key of referencedKeys(readFileSync(file, 'utf8'), known)) referenced.add(key)
  }

  it('defines every key the code names', () => {
    // `t` takes a plain string, so a missing key renders as itself and no type
    // error is raised. This is what makes deleting a key fail here instead.
    // Plural keys are named by their base; i18next appends the suffix at runtime.
    const missing = [...referenced].filter(
      (key) =>
        !known.has(key) && ![...known].some((entry) => entry.replace(PLURAL_SUFFIX, '') === key)
    )
    expect(missing.sort()).toEqual([])
  })

  it('carries no key nothing refers to', () => {
    // A dictionary that only grows is one nobody prunes: a key left behind by a
    // removed feature reads as support for it.
    const unused = [...known].filter(
      (key) =>
        !referenced.has(key) &&
        !referenced.has(key.replace(PLURAL_SUFFIX, '')) &&
        !DYNAMIC_PREFIXES.some((prefix) => key.startsWith(prefix))
    )
    expect(unused.sort()).toEqual([])
  })
})
