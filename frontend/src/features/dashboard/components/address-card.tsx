import { useTranslation } from 'react-i18next'
import { CircleHelp, WifiOff } from 'lucide-react'
import { formatDuration } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Paste } from '@/components/paste'

import { Skeleton } from '@/components/ui/skeleton'

type AddressCardProps = {
  family: 'v4' | 'v6'
  icon: React.ElementType
  ip?: string
  available?: boolean
  message?: string
  /**
   * Seconds since this family last changed, from the change log rather than
   * from a clock started at page load. Null when nothing has been recorded yet.
   */
  sinceSec?: number | null
  loading?: boolean
}

/**
 * The address this host is reachable on, right now.
 *
 * `available === false` is the server saying this family has no egress; it is
 * a settled state, so the card is marked disabled and dimmed. A missing
 * payload is different - the request has not answered - and stays a plain
 * "unknown" so a failure never looks like a disabled feature.
 */
export function AddressCard({
  family,
  icon: Icon,
  ip,
  available,
  message,
  sinceSec,
  loading,
}: AddressCardProps) {
  const { t } = useTranslation()
  const label = family === 'v4' ? t('dashboard.publicIpv4') : t('dashboard.publicIpv6')
  const noEgress = available === false
  const StateIcon = noEgress ? WifiOff : CircleHelp

  return (
    <Card
      aria-disabled={noEgress || undefined}
      data-disabled={noEgress || undefined}
      className={cn(noEgress && 'border-muted bg-muted/30')}
    >
      <CardHeader className='flex flex-row items-center justify-between gap-2 space-y-0'>
        <CardTitle className={cn('text-sm font-medium', noEgress && 'text-muted-foreground')}>
          {label}
        </CardTitle>
        <Icon
          aria-hidden='true'
          className={cn('size-4', noEgress ? 'text-muted-foreground/60' : 'text-muted-foreground')}
        />
      </CardHeader>
      <CardContent className='space-y-1'>
        {loading ? (
          <Skeleton className='h-8 w-52' />
        ) : available && ip ? (
          <>
            <Paste value={ip} label={label} className='[&_code]:text-2xl [&_code]:font-semibold' />
            <p className='text-muted-foreground text-xs'>
              {sinceSec == null
                ? t('dashboard.noChangeYet')
                : t('dashboard.unchangedFor', { duration: formatDuration(sinceSec) })}
            </p>
          </>
        ) : (
          <div
            className='text-muted-foreground flex items-center gap-2 text-sm'
            title={message || undefined}
          >
            <StateIcon aria-hidden='true' className='size-4 shrink-0' />
            <span>
              {noEgress
                ? t('dashboard.noEgress', { family: family === 'v4' ? '4' : '6' })
                : t('dashboard.addressUnknown')}
            </span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
