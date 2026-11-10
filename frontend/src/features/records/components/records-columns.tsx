import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { MoreHorizontal, Pencil, Power, Trash2 } from 'lucide-react'
import { type DnsRecord } from '@/lib/types'
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

export type RecordRowActions = {
  onEdit: (record: DnsRecord) => void
  onToggle: (record: DnsRecord) => void
  onDelete: (record: DnsRecord) => void
}

/**
 * The records one domain holds, which is what DNSPod will answer with.
 *
 * DNSPod's own names ("@" for the apex, its own name for the default line) are
 * shown as it sends them: translating them into a friendlier form would leave anyone
 * comparing against the console unable to match rows up.
 */
export function useRecordsColumns(actions: RecordRowActions): ColumnDef<DnsRecord>[] {
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
      id: 'sub_domain',
      accessorFn: (record) => record.sub_domain,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('records.hostColumn')} />
      ),
      cell: ({ row }) => <span className='truncate font-medium'>{row.original.sub_domain}</span>,
      enableHiding: false,
    },
    {
      id: 'record_type',
      accessorFn: (record) => record.record_type,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('records.typeColumn')} />
      ),
      cell: ({ row }) => (
        <Badge variant='outline' className='font-mono'>
          {row.original.record_type}
        </Badge>
      ),
      meta: { className: 'w-20' },
    },
    {
      id: 'value',
      accessorFn: (record) => record.value,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('records.valueColumn')} />
      ),
      cell: ({ row }) => (
        <span className='block max-w-64 truncate font-mono text-xs'>{row.original.value}</span>
      ),
    },
    {
      id: 'line',
      accessorFn: (record) => record.line,
      header: () => <div className='text-sm font-medium'>{t('records.lineLabel')}</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground truncate text-sm'>{row.original.line}</span>
      ),
      enableSorting: false,
      meta: { className: 'w-32' },
    },
    {
      id: 'ttl',
      accessorFn: (record) => record.ttl,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('records.ttlColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground tabular-nums'>{row.original.ttl}</span>
      ),
      meta: { className: 'w-20' },
    },
    {
      id: 'status',
      accessorFn: (record) => record.status,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('records.statusColumn')} />
      ),
      cell: ({ row }) => (
        <Badge variant={row.original.status === 'ENABLE' ? 'default' : 'secondary'}>
          {row.original.status}
        </Badge>
      ),
      meta: { className: 'w-24' },
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
            <DropdownMenuItem onClick={() => actions.onToggle(row.original)}>
              <Power />
              {row.original.status === 'ENABLE' ? t('records.disable') : t('records.enable')}
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
