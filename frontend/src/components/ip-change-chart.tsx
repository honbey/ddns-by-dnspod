import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip, type ChartConfig } from '@/components/ui/chart'
import { formatDuration, formatTime } from '@/lib/format'
import { type IPChangePoint } from '@/lib/types'

// One colour, taken from the theme: the chart has nothing to encode beyond the
// lifetime, so hue would only be decoration. The label is set at render - the
// series name is interface copy like any other.
const chartColor = 'var(--foreground)'

/**
 * Intervals a person reads without effort. Seconds are what the API returns and
 * what nobody can picture, so the axis picks the first of these that fits and
 * labels them in words instead.
 */
const NICE_STEPS = [
  60, 300, 900, 1800, 3600, 7200, 21600, 43200, 86400, 172800, 604800, 1209600, 2592000,
]

/** At most six ticks, each landing on a step from the list above. */
function niceTicks(maximum: number): number[] {
  if (maximum <= 0) return [60]

  const step = NICE_STEPS.find((candidate) => maximum / candidate <= 5) ?? 2592000
  const ticks: number[] = []
  for (let value = 0; value <= maximum + step; value += step) ticks.push(value)
  return ticks
}

/**
 * An address shortened to fit under an axis.
 *
 * IPv6 in full is longer than the space above a bar is wide, so the middle
 * falls away and the two ends that say what it is are kept: the prefix identifies
 * the network, the last group distinguishes one host from another.
 */
function shortAddress(address: string): string {
  if (address.includes(':')) {
    const groups = address.split(':')
    if (groups.length <= 3) return address
    return `${groups.slice(0, 2).join(':')}:..:${groups[groups.length - 1]}`
  }
  return address
}

type IPChangeChartProps = {
  points: IPChangePoint[]
  /**
   * How many recent changes to plot. A crowd of addresses is unreadable at any
   * angle, so the page shows the newest few rather than everything ever.
   */
  max?: number
}

/**
 * How long each public address lived before it changed.
 *
 * The address is the x axis because that is what the reader recognises; the axis
 * they compare heights against is the lifetime, in words.
 */
export function IPChangeChart({ points, max = 12 }: IPChangeChartProps) {
  const { t } = useTranslation()
  const chartConfig = {
    lifetime: { label: t('dashboard.lifetime'), color: chartColor },
  } satisfies ChartConfig
  const recent = useMemo(() => points.slice(-max), [points, max])

  const data = recent.map((point) => ({
    label: shortAddress(point.new_ip),
    address: point.new_ip,
    lifetime: point.duration_sec ?? 0,
    since: point.time,
    from: point.old_ip,
    family: point.family,
  }))

  const ticks = niceTicks(Math.max(...data.map((point) => point.lifetime), 0))

  if (data.length === 0) {
    return (
      <p className='text-muted-foreground flex h-64 items-center justify-center text-sm'>
        {t('dashboard.addressHistoryEmpty')}
      </p>
    )
  }

  return (
    <ChartContainer config={chartConfig} className='h-full min-h-72 w-full'>
      <BarChart
        data={data}
        margin={{ left: 8, right: 8, top: 8 }}
        barCategoryGap='28%'
        // Off by default in recharts: without it the values exist only in the
        // hover tooltip, which a keyboard or screen-reader user never reaches.
        accessibilityLayer
      >
        <CartesianGrid vertical={false} strokeDasharray='3 3' />
        <XAxis
          dataKey='label'
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          height={64}
          // Every bar is labelled: skipping alternates would hide half the
          // addresses and the point is to read them off directly.
          interval={0}
          angle={-30}
          textAnchor='end'
          tick={{ fontSize: 11 }}
        />
        <YAxis
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          width={56}
          domain={[0, ticks[ticks.length - 1]]}
          ticks={ticks}
          tickFormatter={(value: number) => formatDuration(value)}
          tick={{ fontSize: 11 }}
        />
        <ChartTooltip
          cursor={{ fill: 'var(--muted)', opacity: 0.35 }}
          content={({ active, payload }) => {
            if (!active || !payload?.length) return null
            const point = payload[0]?.payload as
              { address: string; since: string; family: string; lifetime: number } | undefined
            if (!point) return null
            return (
              <div className='bg-popover text-popover-foreground rounded-md border px-3 py-2 text-xs shadow-md'>
                {/* The full address belongs here: the tooltip has the room the axis does not. */}
                <p className='font-medium'>
                  {point.address} (IPv{point.family === 'v6' ? '6' : '4'})
                </p>
                <p className='text-muted-foreground'>since {formatTime(point.since)}</p>
                <p className='mt-1 font-medium'>{formatDuration(point.lifetime)}</p>
              </div>
            )
          }}
        />
        <Bar dataKey='lifetime' fill='var(--color-lifetime)' radius={4} maxBarSize={40} />
      </BarChart>
    </ChartContainer>
  )
}
