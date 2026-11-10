import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Initials from a display name: first character of the first word + first
 * character of the last word. One word only: first two characters. Empty: `?`.
 */
export function getDisplayNameInitials(displayName: string): string {
  const parts = displayName.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) {
    return parts[0].slice(0, 2).toUpperCase()
  }
  const first = parts[0][0] ?? ''
  const last = parts[parts.length - 1]?.[0] ?? ''
  return (first + last).toUpperCase()
}

/**
 * The page a typed value asks for, clamped to the pages the table actually has.
 *
 * Blank, non-numeric and out-of-range-low values keep the current page: the
 * input is a shortcut, not a place to report errors. Values past the end go to
 * the last page, which also keeps a huge number (parseInt may return one past
 * Number.MAX_SAFE_INTEGER) from reaching the table.
 *
 * Both ends are clamped here rather than trusting the caller: a table with no
 * rows still has one page, and an index of -1 is not a page.
 */
export function parsePageTarget(value: string, currentPage: number, totalPages: number): number {
  const lastPage = Math.max(1, totalPages)
  const current = Math.min(Math.max(1, currentPage), lastPage)
  const next = Number.parseInt(value, 10)
  if (Number.isNaN(next) || next < 1) return current
  return Math.min(next, lastPage)
}

/**
 * Whether focus moved into the pager's own navigation group.
 *
 * Used on blur: clicking or tabbing to a page button must not also commit a
 * half-typed page, or one interaction would navigate twice. `relatedTarget`
 * covers mouse, touch and keyboard, and leaves no flag behind to swallow the
 * next legitimate commit.
 */
export function isNavigationTarget(
  relatedTarget: EventTarget | null,
  container: Node | null
): boolean {
  if (!(relatedTarget instanceof Node) || container === null) return false
  return container.contains(relatedTarget)
}
