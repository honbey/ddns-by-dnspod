import { useTranslation } from 'react-i18next'
import { type ColumnDef } from '@tanstack/react-table'
import { MoreHorizontal, Pencil, Send, Trash2 } from 'lucide-react'
import { type NotifyChannel } from '@/lib/types'
import { formatTime } from '@/lib/format'
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
import { CHANNEL_TYPES, channelTypeLabel, channelTarget } from './channel-types'

export type ChannelRowActions = {
  onEdit: (channel: NotifyChannel) => void
  onTest: (channel: NotifyChannel) => void
  /** Resolves when the change has landed, so the switch knows it can let go. */
  onToggle: (channel: NotifyChannel) => Promise<unknown>
  onDelete: (channel: NotifyChannel) => void
}

/** Where a change is announced, in the shape the other tables use. */
export function useChannelsColumns(actions: ChannelRowActions): ColumnDef<NotifyChannel>[] {
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
      id: 'name',
      accessorFn: (channel) => channel.name,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('notifications.nameColumn')} />
      ),
      cell: ({ row }) => <span className='block truncate font-medium'>{row.original.name}</span>,
      enableHiding: false,
    },
    {
      id: 'type',
      accessorFn: (channel) => channel.type,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('notifications.typeColumn')} />
      ),
      cell: ({ row }) => {
        const kind = CHANNEL_TYPES.find((item) => item.id === row.original.type)
        const Icon = kind?.icon
        return (
          <Badge variant='secondary' className='gap-1'>
            {Icon ? <Icon aria-hidden='true' className='size-3' /> : null}
            {channelTypeLabel(row.original.type)}
          </Badge>
        )
      },
      meta: { className: 'w-32' },
    },
    {
      id: 'target',
      accessorFn: (channel) => channelTarget(channel),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('notifications.serverColumn')} />
      ),
      cell: ({ row }) => (
        <span className='block truncate font-mono text-xs'>{channelTarget(row.original)}</span>
      ),
    },
    {
      id: 'updated_at',
      accessorFn: (channel) => channel.updated_at,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('notifications.updatedColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground whitespace-nowrap text-sm'>
          {formatTime(row.original.updated_at)}
        </span>
      ),
    },
    {
      id: 'enabled',
      accessorFn: (channel) => (channel.enabled ? 'on' : 'off'),
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: () => <div className='text-sm font-medium'>{t('status.enabled')}</div>,
      cell: ({ row }) => (
        <RowSwitch
          checked={row.original.enabled}
          onToggle={() => actions.onToggle(row.original)}
          label={`${row.original.enabled ? t('records.disable') : t('records.enable')} ${row.original.name}`}
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
            <DropdownMenuItem onClick={() => actions.onTest(row.original)}>
              <Send />
              {t('notifications.sendTest')}
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
