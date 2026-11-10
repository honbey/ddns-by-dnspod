import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { ArrowRight, CirclePause, TriangleAlert } from 'lucide-react'
import { type HistoryEntry } from '@/lib/types'
import { formatDuration, formatTime, statusVariant } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { DataTableColumnHeader } from '@/components/data-table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

/** The host a change belongs to; an apex has no subdomain to show. */
export function hostOf(entry: HistoryEntry): string {
  return entry.sub_domain ? `${entry.sub_domain}.${entry.domain}` : entry.domain
}

/** The pair as text, so a message that just repeats it can be spotted and dropped. */
function changeOf(entry: HistoryEntry): string {
  return `${entry.old_ip || '-'} -> ${entry.new_ip || '-'}`
}

/**
 * What the tasks have done.
 *
 * Read newest-first and never edited: a row is a fact about the past. The jump
 * is one cell (old -> new, with how long the old address had lasted), because
 * what matters is the transition, not either end of it - and a "took" column
 * reading "5d" would be a lie, since it is a lifetime, not a run time.
 */
export function useHistoryColumns(): ColumnDef<HistoryEntry>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'created_at',
      accessorFn: (entry) => entry.created_at,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('history.timeColumn')} />
      ),
      cell: ({ row }) => (
        <span className='block max-w-40 truncate text-sm'>
          {formatTime(row.original.created_at)}
        </span>
      ),
      meta: { className: 'w-40 whitespace-nowrap' },
    },
    {
      id: 'host',
      accessorFn: hostOf,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('history.hostColumn')} />
      ),
      cell: ({ row }) => {
        const entry = row.original
        // The detail is a hover away rather than a column: most rows have
        // nothing to say, and an empty column is all padding.
        const message = entry.message?.trim()
        const detail =
          !message || message === changeOf(entry) || entry.status === 'OK' ? null : message
        return (
          <div className='min-w-0'>
            <div className='flex items-center gap-1.5'>
              <span className='truncate font-medium'>{hostOf(entry)}</span>
              {detail && entry.status === 'FAIL' ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    {/* A button, because the icon on its own cannot be focused:
                        the reason was reachable with a mouse and nothing else. */}
                    <button
                      type='button'
                      className='shrink-0 rounded-sm focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none'
                    >
                      <TriangleAlert aria-hidden='true' className='size-3.5 text-destructive' />
                      <span className='sr-only'>
                        {t('ui.whyFailed')}
                        {detail}
                      </span>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent className='max-w-md break-words'>{detail}</TooltipContent>
                </Tooltip>
              ) : null}
              {detail && entry.status === 'SKIP' ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type='button'
                      className='shrink-0 rounded-sm focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none'
                    >
                      <CirclePause aria-hidden='true' className='text-muted-foreground size-3.5' />
                      <span className='sr-only'>
                        {t('ui.whySkipped')}
                        {detail}
                      </span>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>{detail}</TooltipContent>
                </Tooltip>
              ) : null}
            </div>
          </div>
        )
      },
      enableHiding: false,
    },
    {
      id: 'record_type',
      accessorFn: (entry) => entry.record_type,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('history.typeColumn')} />
      ),
      cell: ({ row }) => (
        <Badge variant='outline' className='font-mono'>
          {row.original.record_type}
        </Badge>
      ),
      meta: { className: 'w-20' },
    },
    {
      id: 'domain',
      accessorFn: (entry) => entry.domain,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: () => <div className='text-sm font-medium'>{t('history.domainColumn')}</div>,
      cell: ({ row }) => <span>{row.original.domain}</span>,
    },
    {
      id: 'change',
      header: () => <div className='text-sm font-medium'>{t('history.changeColumn')}</div>,
      cell: ({ row }) => (
        <div
          className='flex items-center gap-1 font-mono text-xs'
          title={
            row.original.duration_sec != null
              ? `previous address lasted ${formatDuration(row.original.duration_sec)}`
              : undefined
          }
        >
          <span className='text-muted-foreground truncate'>{row.original.old_ip || '-'}</span>
          <ArrowRight aria-hidden='true' className='size-3 shrink-0' />
          <span className='truncate'>{row.original.new_ip || '-'}</span>
        </div>
      ),
      enableSorting: false,
    },
    {
      id: 'status',
      accessorFn: (entry) => entry.status,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('history.resultColumn')} />
      ),
      cell: ({ row }) => (
        <Badge variant={statusVariant(row.original.status)}>{row.original.status}</Badge>
      ),
      meta: { className: 'w-24' },
    },
  ]
}
