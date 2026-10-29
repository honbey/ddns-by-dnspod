import { useTranslation } from 'react-i18next'
import { type LucideIcon } from 'lucide-react'
import { formatDuration } from '@/lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Paste } from '@/components/paste'

import { Skeleton } from '@/components/ui/skeleton'

type AddressCardProps = {
  family: 'v4' | 'v6'
  icon: LucideIcon
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
 * It leads the page because it is the one thing every task exists to chase:
 * the number a person checks first, and the one they paste somewhere next.
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

  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between gap-2 space-y-0'>
        <CardTitle className='text-sm font-medium'>{label}</CardTitle>
        <Icon aria-hidden='true' className='text-muted-foreground size-4' />
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
          <p className='text-muted-foreground text-sm'>
            {message || t('dashboard.noEgress', { family: family === 'v4' ? '4' : '6' })}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
