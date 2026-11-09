import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { ArrowRight } from 'lucide-react'
import { type IPChange } from '@/lib/types'
import { formatDuration, formatTime } from '@/lib/format'
import { Badge } from '@/components/ui/badge'

/**
 * What this host's public address did, newest first.
 *
 * The first row of a family has no previous address, so its change cell shows
 * a dash rather than an arrow from nowhere - and it is still listed, because
 * "the address was first seen here" is the fact the rest of the timeline is
 * measured from.
 *
 * No column is client sortable: the page holds one slice of a server ordered
 * timeline, and sorting that slice would read as if it were the whole table.
 */
export function useIPHistoryColumns(): ColumnDef<IPChange>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'created_at',
      accessorFn: (change) => change.created_at,
      header: () => <div className='text-sm font-medium'>{t('history.timeColumn')}</div>,
      cell: ({ row }) => (
        <span className='block max-w-40 truncate text-sm'>
          {formatTime(row.original.created_at)}
        </span>
      ),
      enableSorting: false,
      meta: { className: 'w-40 whitespace-nowrap' },
    },
    {
      id: 'family',
      accessorFn: (change) => change.family,
      header: () => <div className='text-sm font-medium'>{t('history.ipFamilyColumn')}</div>,
      cell: ({ row }) => (
        <Badge variant='outline' className='font-mono'>
          IPv{row.original.family === 'v6' ? '6' : '4'}
        </Badge>
      ),
      enableSorting: false,
      meta: { className: 'w-24' },
    },
    {
      id: 'change',
      header: () => <div className='text-sm font-medium'>{t('history.changeColumn')}</div>,
      cell: ({ row }) => (
        <div className='flex items-center gap-1 font-mono text-xs'>
          <span className='text-muted-foreground truncate'>{row.original.old_ip || '-'}</span>
          <ArrowRight aria-hidden='true' className='size-3 shrink-0' />
          <span className='truncate'>{row.original.new_ip || '-'}</span>
        </div>
      ),
      enableSorting: false,
    },
    {
      id: 'duration_sec',
      accessorFn: (change) => change.duration_sec ?? null,
      header: () => <div className='text-sm font-medium'>{t('history.ipDurationColumn')}</div>,
      cell: ({ row }) => (
        <span className='text-sm'>{formatDuration(row.original.duration_sec)}</span>
      ),
      enableSorting: false,
      meta: { className: 'w-32' },
    },
  ]
}
