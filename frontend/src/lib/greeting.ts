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
