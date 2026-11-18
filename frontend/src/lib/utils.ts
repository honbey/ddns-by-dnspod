import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * The letter a person's avatar shows: the first character of the display name.
 *
 * One letter, not initials: the circle is 36px, and two characters crowd it
 * without telling anyone more than the name beside it already does.
 */
export function getDisplayNameInitial(displayName: string): string {
  // Spread rather than [0], so a name that starts with an emoji is not cut in
  // half and rendered as a replacement character.
  const first = [...displayName.trim()][0]
  return first ? first.toUpperCase() : '?'
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
