import { useState } from 'react'
import {
  type SortingState,
  type VisibilityState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { Power, PowerOff, Send, Trash2 } from 'lucide-react'
import { type NotifyChannel } from '@/lib/types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  DataTableBulkActions,
  DataTablePagination,
  DataTableToolbar,
} from '@/components/data-table'
import { useTranslation } from 'react-i18next'
import { useChannelsColumns, type ChannelRowActions } from './channels-columns'
import { CHANNEL_TYPES, channelTarget } from './channel-types'

type ChannelsTableProps = {
  data: NotifyChannel[]
  loading: boolean
  actions: ChannelRowActions
  onBulkTest: (channels: NotifyChannel[]) => void
  onBulkSetEnabled: (channels: NotifyChannel[], enabled: boolean) => void
  onBulkDelete: (channels: NotifyChannel[]) => void
}

/** Where a change is announced. */
export function ChannelsTable({
  data,
  loading,
  actions,
  onBulkTest,
  onBulkSetEnabled,
  onBulkDelete,
}: ChannelsTableProps) {
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([{ id: 'name', desc: false }])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [globalFilter, setGlobalFilter] = useState('')

  const { t } = useTranslation()
  const columns = useChannelsColumns(actions)
  const typeOptions = CHANNEL_TYPES.map((type) => ({
    label: type.label,
    value: type.id,
  }))
  const stateOptions = [
    { label: t('status.enabled'), value: 'on' },
    { label: t('status.disabled'), value: 'off' },
  ]

  const table = useReactTable({
    data,
    columns,
    // Keyed by the channel id rather than the row index, so a refetch after a
    // delete cannot leave the selection pointing at a different channel.
    getRowId: (row) => String(row.id),
    state: { sorting, columnVisibility, globalFilter, rowSelection },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = String(filterValue).toLowerCase()
      const channel = row.original
      return [channel.name, channel.type, channelTarget(channel)]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  const selectedRows = table.getFilteredSelectedRowModel().rows

  return (
    <div className={cn('max-sm:has-[div[role="toolbar"]]:mb-16', 'flex flex-1 flex-col gap-4')}>
      <DataTableToolbar
        table={table}
        searchPlaceholder={t('table.filter')}
        filters={[
          {
            columnId: 'type',
            title: t('notifications.typeColumn'),
            options: typeOptions,
          },
          {
            columnId: 'enabled',
            title: t('tasks.stateColumn'),
            options: stateOptions,
          },
        ]}
      />
      <div className='overflow-hidden rounded-md border'>
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    colSpan={header.colSpan}
                    // The sort is drawn as an arrow by the header button, which
                    // a screen reader never sees.
                    aria-sort={
                      header.column.getIsSorted() === 'asc'
                        ? 'ascending'
                        : header.column.getIsSorted() === 'desc'
                          ? 'descending'
                          : undefined
                    }
                    className={cn(
                      header.column.columnDef.meta?.className,
                      header.column.columnDef.meta?.thClassName
                    )}
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 3 }).map((_, rowIndex) => (
                <TableRow key={`skeleton-${rowIndex}`}>
                  {columns.map((_column, cellIndex) => (
                    <TableCell key={`skeleton-cell-${cellIndex}`}>
                      <Skeleton className='h-5 w-full' />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : table.getRowModel().rows.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell
                      key={cell.id}
                      className={cn(
                        cell.column.columnDef.meta?.className,
                        cell.column.columnDef.meta?.tdClassName
                      )}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className='text-muted-foreground h-24 text-center'
                >
                  {data.length === 0 ? t('notifications.empty') : t('common.nothingMatches')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} className='mt-auto' />
      <DataTableBulkActions table={table} entityName={t('entity.channel')}>
        <Button
          variant='outline'
          size='sm'
          onClick={() => onBulkTest(selectedRows.map((row) => row.original))}
        >
          <Send />
          {t('notifications.sendTest')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() =>
            onBulkSetEnabled(
              selectedRows.map((row) => row.original),
              true
            )
          }
        >
          <Power />
          {t('records.enable')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() =>
            onBulkSetEnabled(
              selectedRows.map((row) => row.original),
              false
            )
          }
        >
          <PowerOff />
          {t('records.disable')}
        </Button>
        <Button
          variant='destructive'
          size='sm'
          onClick={() => onBulkDelete(selectedRows.map((row) => row.original))}
        >
          <Trash2 />
          {t('common.delete')}
        </Button>
      </DataTableBulkActions>
    </div>
  )
}
