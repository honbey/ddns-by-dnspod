import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { type HistoryEntry } from '@/lib/types'
import { formatTime, statusVariant } from '@/lib/format'
import { getDisplayNameInitials } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'

type RecentChangesProps = {
  /** The newest entries, already cut to what the card will scroll through. */
  entries: HistoryEntry[]
  loading: boolean
  /** How many updates exist in total, which is what the description counts. */
  total: number
}

/**
 * What the tasks did, as a list rather than a table.
 *
 * The card scrolls instead of growing: the chart beside it sets the height of
 * the row, and a list that kept pushing it down would drag the chart taller
 * every time history accumulated.
 */
export function RecentChanges({ entries, loading, total }: RecentChangesProps) {
  const { t } = useTranslation()
  return (
    <Card className='flex h-full flex-col'>
      <CardHeader>
        <CardTitle>{t('dashboard.latestChanges')}</CardTitle>
        <CardDescription>
          {total === 0
            ? t('dashboard.nothingChanged')
            : entries.length < total
              ? t('dashboard.newestOf', { shown: entries.length, total })
              : total === 1
                ? t('dashboard.oneUpdateRecorded')
                : t('dashboard.updatesRecorded', { total })}
        </CardDescription>
        <CardAction>
          <Button asChild variant='ghost' size='sm'>
            <Link to='/history'>{t('dashboard.seeAll')}</Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='min-h-0 flex-1'>
        {loading ? (
          <div className='space-y-6'>
            {Array.from({ length: 5 }).map((_, index) => (
              <div key={index} className='flex items-center gap-3'>
                <Skeleton className='size-9 rounded-full' />
                <div className='flex-1 space-y-2'>
                  <Skeleton className='h-4 w-32' />
                  <Skeleton className='h-3 w-24' />
                </div>
              </div>
            ))}
          </div>
        ) : entries.length === 0 ? (
          <p className='text-muted-foreground flex h-64 items-center justify-center text-sm'>
            {t('dashboard.nothingRecorded')}
          </p>
        ) : (
          <ul className='max-h-88 space-y-4 overflow-y-auto pe-1'>
            {entries.map((entry) => {
              const host = entry.sub_domain ? `${entry.sub_domain}.${entry.domain}` : entry.domain
              return (
                <li key={entry.id} className='flex items-center gap-3'>
                  <Avatar className='size-9'>
                    <AvatarFallback>{getDisplayNameInitials(host)}</AvatarFallback>
                  </Avatar>
                  <div className='min-w-0 flex-1 space-y-0.5'>
                    <p className='truncate text-sm font-medium'>{host}</p>
                    <p className='text-muted-foreground truncate text-xs'>
                      {entry.record_type} - {formatTime(entry.created_at)}
                    </p>
                  </div>
                  <div className='shrink-0 space-y-0.5 text-end'>
                    <p className='truncate font-mono text-xs'>{entry.new_ip || '-'}</p>
                    <Badge variant={statusVariant(entry.status)}>{entry.status}</Badge>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
