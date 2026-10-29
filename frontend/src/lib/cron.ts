/** Schedules and the DNSPod options shared by the task dialog and its tables. */

export const cronPresets = [
  { label: 'Every minute', value: '* * * * *' },
  { label: 'Every 5 minutes', value: '*/5 * * * *' },
  { label: 'Every 10 minutes', value: '*/10 * * * *' },
  { label: 'Every 30 minutes', value: '*/30 * * * *' },
  { label: 'Every hour', value: '0 * * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Once a day', value: '0 3 * * *' },
]

/** Record types that point at an address; DNSPod has others, DDNS cannot use them. */
export const RECORD_TYPES = ['A', 'AAAA'] as const

/** The TTLs DNSPod offers most hosts, labelled in words rather than seconds. */
export const TTL_OPTIONS = [
  { label: '10 minutes', value: 600 },
  { label: '30 minutes', value: 1800 },
  { label: '1 hour', value: 3600 },
  { label: '6 hours', value: 21600 },
  { label: '12 hours', value: 43200 },
  { label: '1 day', value: 86400 },
]

/** The bounds of each field: minute, hour, day of month, month, day of week. */
const CRON_FIELDS = [
  { min: 0, max: 59 },
  { min: 0, max: 23 },
  { min: 1, max: 31 },
  { min: 1, max: 12 },
  { min: 0, max: 7 },
] as const

/** The shorthand the parser takes instead of five fields. */
const CRON_DESCRIPTOR = /^@(annually|yearly|monthly|weekly|daily|midnight|hourly)$/
const CRON_EVERY = /^@every\s+\d+(ns|us|µs|ms|s|m|h)(\d+(ns|us|µs|ms|s|m|h))*$/

/**
 * Whether the scheduler's parser will accept this expression.
 *
 * Each field is taken apart rather than matched against one pattern, because a
 * pattern cannot tell a minute of 99, a step of zero or a range that runs
 * backwards from a real schedule: all three look like cron and none of them is
 * one. The descriptors are accepted because the parser accepts them, so what
 * this calls valid is what the server will take.
 */
export function isValidCron(expr: string): boolean {
  const trimmed = expr.trim()
  if (trimmed.startsWith('@')) {
    return CRON_DESCRIPTOR.test(trimmed) || CRON_EVERY.test(trimmed)
  }

  const fields = trimmed.split(/\s+/)
  if (fields.length !== CRON_FIELDS.length) return false

  return fields.every((field, index) => {
    // Names are only a thing for the month and the day of the week
    const named = index === 3 || index === 4
    const { min, max } = CRON_FIELDS[index]

    return field.split(',').every((item) => {
      const [range, step, rest] = item.split('/')
      if (rest !== undefined) return false
      // A step of zero would divide the field by nothing
      if (step !== undefined && !/^\d+$/.test(step)) return false
      if (step !== undefined && Number(step) < 1) return false
      if (range === '*') return true
      if (range === '') return false

      const numbers: number[] = []
      for (const part of range.split('-')) {
        if (/^\d+$/.test(part)) {
          numbers.push(Number(part))
          continue
        }
        if (!named || !/^[a-z]{3}$/i.test(part)) return false
      }
      if (range.split('-').length > 2) return false
      // A range that runs backwards is not one the parser reads
      if (numbers.length === 2 && numbers[0] > numbers[1]) return false
      return numbers.every((value) => value >= min && value <= max)
    })
  })
}

/**
 * What a cron expression means, for the benefit of anyone who has not read one
 * this year. Unrecognised expressions are shown as written rather than guessed at.
 */
export function describeCron(expr: string): string {
  const trimmed = expr.trim()
  const preset = cronPresets.find((option) => option.value === trimmed)
  return preset ? preset.label : trimmed
}
