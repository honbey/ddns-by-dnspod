import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { formatTime, statusVariant } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

type LastRunCellProps = {
  /**
   * When the last run happened; null or missing means it never ran. Tasks pass
   * their last run, certificates the moment the files were last written.
   */
  at?: string | null
  /** The status of that run; empty means there is none yet. */
  status: string
  /**
   * What the run answered, behind the badge: a failure explains itself there
   * rather than taking a third line in every row of the list.
   */
  message?: string
  /** Consecutive failures, shown beside the badge when there are any. */
  failCount?: number
}

/**
 * The last run of one row, presented the same way in every table.
 *
 * Two lines: when it ran, and how it went. Tasks and certificates both answer
 * that question, and one shape keeps the two lists from drifting into different
 * orderings of the same facts.
 */
export function LastRunCell({ at, status, message, failCount = 0 }: LastRunCellProps) {
  const { t } = useTranslation()
  return (
    <div className='min-w-0 space-y-0.5'>
      <div className='truncate text-sm'>
        {at ? formatTime(at) : <span className='text-muted-foreground'>{t('ui.neverRun')}</span>}
      </div>
      {status ? (
        <div className='truncate text-xs'>
          {message ? (
            <Tooltip>
              <TooltipTrigger asChild>
                {/* Focusable: a title attribute would keep the only account of
                    a failure away from the keyboard. */}
                <span
                  tabIndex={0}
                  className='rounded-sm focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none'
                >
                  <Badge variant={statusVariant(status)}>
                    {status === 'RUNNING' ? (
                      <Loader2 aria-hidden='true' className='size-3 animate-spin' />
                    ) : null}
                    {status}
                  </Badge>
                </span>
              </TooltipTrigger>
              <TooltipContent className='max-w-xs' side='top'>
                {message}
              </TooltipContent>
            </Tooltip>
          ) : (
            <Badge variant={statusVariant(status)}>
              {status === 'RUNNING' ? (
                <Loader2 aria-hidden='true' className='size-3 animate-spin' />
              ) : null}
              {status}
            </Badge>
          )}
          {failCount > 0 ? (
            <span className='text-muted-foreground ms-1'>
              {t('ui.failedInRow', { count: failCount })}
            </span>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
