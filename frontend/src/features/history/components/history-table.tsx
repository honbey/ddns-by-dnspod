import { useState } from 'react'
import {
  type SortingState,
  type VisibilityState,
  flexRender,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { type HistoryEntry } from '@/lib/types'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DataTablePagination, DataTableToolbar, type FacetedOption } from '@/components/data-table'
import { useTranslation } from 'react-i18next'
import { useHistoryColumns } from './history-columns'

/**
 * SKIP is never in the data: the server does not record runs where the address
 * did not change, so offering it as a filter would only ever answer with empty.
 */
const STATUS_OPTIONS: FacetedOption[] = [
  { label: 'OK', value: 'OK' },
  { label: 'FAIL', value: 'FAIL' },
]

type HistoryTableProps = {
  data: HistoryEntry[]
  loading: boolean
  domainOptions: FacetedOption[]
}

export function HistoryTable({ data, loading, domainOptions }: HistoryTableProps) {
  const [sorting, setSorting] = useState<SortingState>([{ id: 'created_at', desc: true }])
  // 'domain' stays available to the filter without taking a column of its own
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({
    domain: false,
  })
  const [globalFilter, setGlobalFilter] = useState('')

  const { t } = useTranslation()
  const columns = useHistoryColumns()

  const table = useReactTable({
    data,
    columns,
    // Keyed by the entry id rather than the row index, so a refetch or a new
    // sort does not change the identity of every row.
    getRowId: (row) => String(row.id),
    state: { sorting, columnVisibility, globalFilter },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = String(filterValue).toLowerCase()
      const entry = row.original
      return [entry.domain, entry.sub_domain, entry.old_ip, entry.new_ip, entry.message]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    },
    getCoreRowModel: getCoreRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  return (
    <div className='flex flex-1 flex-col gap-4'>
      <DataTableToolbar
        table={table}
        searchPlaceholder={t('table.filter')}
        filters={[
          {
            columnId: 'domain',
            title: t('history.domainColumn'),
            options: domainOptions,
          },
          {
            columnId: 'status',
            title: t('history.resultColumn'),
            options: STATUS_OPTIONS,
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
              Array.from({ length: 6 }).map((_, rowIndex) => (
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
                  {data.length === 0 ? t('history.empty') : t('common.nothingMatches')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} className='mt-auto' />
    </div>
  )
}
