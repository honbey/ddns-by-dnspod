/**
 * The dashboard subtitle greets by the hour of the local clock. Every branch
 * names a whole dictionary key: a key assembled from a suffix at runtime would
 * be invisible to the dictionary coverage test and read as unused.
 */
export type GreetingKey =
  | 'dashboard.greeting.morning'
  | 'dashboard.greeting.afternoon'
  | 'dashboard.greeting.evening'
  | 'dashboard.greeting.night'

/** Local hour, 0 to 23. */
export function greetingKey(hour: number): GreetingKey {
  if (hour >= 5 && hour < 12) return 'dashboard.greeting.morning'
  if (hour >= 12 && hour < 18) return 'dashboard.greeting.afternoon'
  if (hour >= 18 && hour < 23) return 'dashboard.greeting.evening'
  return 'dashboard.greeting.night'
}

/** A full period to wait when the next boundary cannot be trusted. */
const HOUR_MS = 60 * 60_000

/**
 * Milliseconds from `now` to the next whole local hour.
 *
 * Building the target with setHours rather than adding a fixed hour keeps the
 * boundary exact after a DST step, and stops a repeating timer from drifting a
 * few milliseconds further away on every tick. A clock stepped back past the
 * boundary leaves no useful target: waiting a whole hour beats a 1ms timeout
 * loop that would spin until the clock caught up.
 */
export function msUntilNextHour(now: Date): number {
  const next = new Date(now)
  next.setHours(now.getHours() + 1, 0, 0, 0)
  const delta = next.getTime() - now.getTime()
  return delta > 0 ? delta : HOUR_MS
}
