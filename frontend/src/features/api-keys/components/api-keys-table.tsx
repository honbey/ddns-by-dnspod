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
import { Power, PowerOff, Trash2 } from 'lucide-react'
import { type APIKey } from '@/lib/types'
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
import { useApiKeysColumns, type APIKeyRowActions } from './api-keys-columns'

type APIKeysTableProps = {
  data: APIKey[]
  loading: boolean
  actions: APIKeyRowActions
  accountNames: Map<number, string>
  onBulkSetEnabled: (keys: APIKey[], enabled: boolean) => void
  onBulkDelete: (keys: APIKey[]) => void
}

/** Credentials issued to scripts. */
export function APIKeysTable({
  data,
  loading,
  actions,
  accountNames,
  onBulkSetEnabled,
  onBulkDelete,
}: APIKeysTableProps) {
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([{ id: 'name', desc: false }])
  // 'enabled' is the column id; 'state' matched nothing, so the column stayed
  // visible and the toggle in the toolbar never reflected it.
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({
    enabled: false,
  })
  const [globalFilter, setGlobalFilter] = useState('')

  const { t } = useTranslation()
  const columns = useApiKeysColumns(actions, accountNames)
  const stateOptions = [
    { label: t('status.enabled'), value: 'on' },
    { label: t('status.disabled'), value: 'off' },
  ]

  const table = useReactTable({
    data,
    columns,
    // Keyed by the key id rather than the row index, so a refetch after a
    // delete cannot leave the selection pointing at a different key.
    getRowId: (row) => String(row.id),
    state: { sorting, columnVisibility, globalFilter, rowSelection },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = String(filterValue).toLowerCase()
      const key = row.original
      return [key.name, key.domain, key.key_prefix].join(' ').toLowerCase().includes(needle)
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
                  {data.length === 0 ? t('apiKeys.empty') : t('common.nothingMatches')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} className='mt-auto' />
      <DataTableBulkActions table={table} entityName={t('entity.key')}>
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
