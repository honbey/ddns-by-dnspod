/** Schedules and the DNSPod options shared by the task dialog and its tables. */
import { type TFunction } from 'i18next'
import { type MessageKey } from '@/i18n'

/** A shortcut offered beside a schedule field: a dictionary key and the expression. */
export type CronPreset = { key: MessageKey; value: string }

/**
 * The shortcuts a DDNS task offers. Minutes, because that is the unit a record
 * following an address moves in.
 */
export const taskCronPresets: CronPreset[] = [
  { key: 'cron.everyMinute', value: '* * * * *' },
  { key: 'cron.every5Minutes', value: '*/5 * * * *' },
  { key: 'cron.every10Minutes', value: '*/10 * * * *' },
  { key: 'cron.every30Minutes', value: '*/30 * * * *' },
  { key: 'cron.hourly', value: '0 * * * *' },
]

/**
 * The shortcuts a certificate offers, at the hour this deployment checks.
 *
 * Days, because a ninety-day certificate is looked at a few times rather than
 * continuously; the hour is the one `acme.check_hour` gives new certificates,
 * so a shortcut does not quietly move the schedule to a different hour.
 */
export function certificateCronPresets(hour: number): CronPreset[] {
  return [
    { key: 'cron.daily', value: `0 ${hour} * * *` },
    { key: 'cron.every7Days', value: `0 ${hour} */7 * *` },
    { key: 'cron.monthly', value: `0 ${hour} 1 * *` },
  ]
}

/** A clock hour that the shape tests accept. */
function isHour(field: string | undefined): boolean {
  if (!field || !/^\d{1,2}$/.test(field)) return false
  const hour = Number(field)
  return hour >= 0 && hour <= 23
}

/**
 * The certificate schedules, recognised whatever hour they name.
 *
 * They are shapes rather than fixed values because the hour follows the
 * deployment: `0 3 * * *` and `0 5 * * *` are the same schedule to a reader,
 * and a table column should say "every day" for both.
 */
const CRON_SHAPES: { key: MessageKey; match: (fields: string[]) => boolean }[] = [
  {
    key: 'cron.daily',
    match: ([minute, hour, dom, month, dow]) =>
      minute === '0' && isHour(hour) && dom === '*' && month === '*' && dow === '*',
  },
  {
    key: 'cron.every7Days',
    match: ([minute, hour, dom, month, dow]) =>
      minute === '0' && isHour(hour) && dom === '*/7' && month === '*' && dow === '*',
  },
  {
    // Not "every 30 days": day-of-month 1 is 28-31 days apart, and the label
    // says the day rather than a step it does not keep.
    key: 'cron.monthly',
    match: ([minute, hour, dom, month, dow]) =>
      minute === '0' && isHour(hour) && dom === '1' && month === '*' && dow === '*',
  },
]

/** Record types that point at an address; DNSPod has others, DDNS cannot use them. */
export const RECORD_TYPES = ['A', 'AAAA'] as const

/** The TTLs DNSPod offers most hosts, labelled in words rather than seconds. */
export const TTL_OPTIONS: { key: MessageKey; value: number }[] = [
  { key: 'ttl.minutes10', value: 600 },
  { key: 'ttl.minutes30', value: 1800 },
  { key: 'ttl.hour1', value: 3600 },
  { key: 'ttl.hours6', value: 21600 },
  { key: 'ttl.hours12', value: 43200 },
  { key: 'ttl.day1', value: 86400 },
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
 * A step of one schedules the same as a wildcard; the presets spell it short.
 *
 * Only the star form is rewritten: a range that spans its whole field means the
 * same thing, and is left as written rather than half-recognised.
 */
function withoutUnitSteps(expr: string): string {
  return expr
    .split(/\s+/)
    .map((field) => field.replace(/^\*\/(?:0*1)$/, '*'))
    .join(' ')
}

/**
 * What a cron expression means, for the benefit of anyone who has not read one
 * this year. Unrecognised expressions are shown as written rather than guessed at.
 */
export function describeCron(expr: string, t: TFunction): string {
  const trimmed = expr.trim()
  const normalised = withoutUnitSteps(trimmed)

  const preset = taskCronPresets.find((option) => option.value === normalised)
  if (preset) return t(preset.key)

  const shape = CRON_SHAPES.find((option) => option.match(normalised.split(/\s+/)))
  return shape ? t(shape.key) : trimmed
}
