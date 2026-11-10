import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { MoreHorizontal, Pencil, Play, Trash2 } from 'lucide-react'
import { type DDNSTask } from '@/lib/types'
import { describeCron } from '@/lib/cron'
import { formatDuration, formatTime, statusVariant } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTableColumnHeader } from '@/components/data-table'
import { RowSwitch } from '@/components/data-table/row-switch'

export type TaskRowActions = {
  onEdit: (task: DDNSTask) => void
  onRun: (task: DDNSTask) => void
  /** Resolves when the change has landed, so the switch knows it can let go. */
  onToggle: (task: DDNSTask) => Promise<unknown>
  onDelete: (task: DDNSTask) => void
}

/** The host a task keeps up to date, which is how people look tasks up. */
export function hostOf(task: DDNSTask): string {
  return task.sub_domain ? `${task.sub_domain}.${task.domain}` : task.domain
}

export function useTasksColumns(
  actions: TaskRowActions,
  accountNames: Map<number, string>
): ColumnDef<DDNSTask>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={
            table.getIsAllPageRowsSelected() ||
            (table.getIsSomePageRowsSelected() && 'indeterminate')
          }
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('common.selectAll')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label={t('common.selectRow')}
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { className: 'w-8' },
    },
    {
      id: 'host',
      accessorFn: hostOf,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('tasks.hostColumn')} />
      ),
      cell: ({ row }) => (
        <div className='min-w-0'>
          <div className='truncate font-medium'>{hostOf(row.original)}</div>
          <div className='text-muted-foreground truncate text-xs'>
            {row.original.record_line || t('ui.defaultLine')}
          </div>
        </div>
      ),
      enableHiding: false,
    },
    {
      id: 'record_type',
      accessorFn: (task) => task.record_type,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('tasks.typeColumn')} />
      ),
      cell: ({ row }) => (
        <Badge variant='outline' className='font-mono'>
          {row.original.record_type}
        </Badge>
      ),
      meta: { className: 'w-20' },
    },
    {
      id: 'account',
      accessorFn: (task) => accountNames.get(task.account_id) ?? `#${task.account_id}`,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('tasks.accountColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground truncate text-sm'>
          {accountNames.get(row.original.account_id) ?? `#${row.original.account_id}`}
        </span>
      ),
    },
    {
      id: 'address',
      accessorFn: (task) => task.current_ip,
      header: () => <div className='text-sm font-medium'>{t('tasks.addressColumn')}</div>,
      cell: ({ row }) => (
        <div className='min-w-0'>
          <div className='truncate font-mono text-xs'>{row.original.current_ip || '-'}</div>
          {row.original.ip_since ? (
            <div className='text-muted-foreground truncate text-xs'>
              {t('ui.duration', {
                duration: formatDuration(
                  Math.floor((Date.now() - new Date(row.original.ip_since).getTime()) / 1000)
                ),
              })}
            </div>
          ) : null}
        </div>
      ),
      enableSorting: false,
    },
    {
      id: 'cron_expr',
      accessorFn: (task) => task.cron_expr,
      header: () => <div className='text-sm font-medium'>{t('tasks.scheduleLabel')}</div>,
      cell: ({ row }) => (
        <div className='min-w-0'>
          <div className='truncate'>{describeCron(row.original.cron_expr)}</div>
          <div className='text-muted-foreground truncate font-mono text-xs'>
            {row.original.cron_expr}
          </div>
        </div>
      ),
      enableSorting: false,
    },
    {
      id: 'last_run',
      accessorFn: (task) => task.last_run_at ?? '',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('tasks.lastRunColumn')} />
      ),
      cell: ({ row }) => {
        const task = row.original
        return (
          <div className='min-w-0'>
            {task.last_run_at ? (
              <>
                <div className='truncate text-sm'>{formatTime(task.last_run_at)}</div>
                <div className='truncate text-xs'>
                  <Badge variant={statusVariant(task.last_status)}>
                    {task.last_status || t('ui.neverRun')}
                  </Badge>
                  {task.fail_count > 0 ? (
                    <span className='text-muted-foreground ms-1'>
                      {t('ui.failedInRow', { count: task.fail_count })}
                    </span>
                  ) : null}
                </div>
              </>
            ) : (
              <span className='text-muted-foreground text-sm'>{t('ui.neverRun')}</span>
            )}
          </div>
        )
      },
    },
    {
      id: 'enabled',
      accessorFn: (task) => (task.enabled ? 'enabled' : 'paused'),
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: () => <div className='text-sm font-medium'>{t('status.enabled')}</div>,
      cell: ({ row }) => (
        <RowSwitch
          checked={row.original.enabled}
          onToggle={() => actions.onToggle(row.original)}
          label={t(row.original.enabled ? 'tasks.pause' : 'tasks.resume', {
            host: hostOf(row.original),
          })}
        />
      ),
      enableSorting: false,
      meta: { className: 'w-20' },
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger asChild>
            <Button variant='ghost' size='icon' className='size-8'>
              <MoreHorizontal className='size-4' />
              <span className='sr-only'>{t('common.openMenu')}</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-44'>
            <DropdownMenuItem onClick={() => actions.onEdit(row.original)}>
              <Pencil />
              {t('common.edit')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => actions.onRun(row.original)}>
              <Play />
              {t('tasks.runNow')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant='destructive' onClick={() => actions.onDelete(row.original)}>
              <Trash2 />
              {t('common.delete')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { className: 'w-10' },
    },
  ]
}
