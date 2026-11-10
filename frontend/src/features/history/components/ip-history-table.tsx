import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type PaginationState,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { type IPFamily } from '@/features/history/api'
import { type IPChange } from '@/lib/types'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DataTablePagination } from '@/components/data-table'
import { IPHistoryImport } from './ip-history-import'
import { useIPHistoryColumns } from './ip-history-columns'

const ALL_FAMILIES = 'all'

type IPHistoryTableProps = {
  data: IPChange[]
  total: number
  page: number
  pageSize: number
  family: IPFamily | undefined
  loading: boolean
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  onFamilyChange: (family: IPFamily | undefined) => void
}

/**
 * The table half of the address timeline.
 *
 * Pagination is the server's: the timeline has no natural upper bound once a
 * CSV is imported, and a client window would silently stop at whatever the
 * first request returned. The family filter is a query parameter for the same
 * reason - filtering one page would answer about that page, not the history.
 */
export function IPHistoryTable({
  data,
  total,
  page,
  pageSize,
  family,
  loading,
  onPageChange,
  onPageSizeChange,
  onFamilyChange,
}: IPHistoryTableProps) {
  const { t } = useTranslation()
  const columns = useIPHistoryColumns()

  const table = useReactTable({
    data,
    columns,
    // Keyed by the entry id rather than the row index, so a page change or a
    // refetch does not change the identity of every row.
    getRowId: (row) => String(row.id),
    manualPagination: true,
    pageCount: Math.max(1, Math.ceil(total / pageSize)),
    state: { pagination: { pageIndex: page - 1, pageSize } },
    onPaginationChange: (updater) => {
      const current: PaginationState = { pageIndex: page - 1, pageSize }
      const next = typeof updater === 'function' ? updater(current) : updater
      if (next.pageSize !== current.pageSize) {
        onPageSizeChange(next.pageSize)
        return
      }
      onPageChange(next.pageIndex + 1)
    },
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <div className='flex flex-1 flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <Select
          value={family ?? ALL_FAMILIES}
          onValueChange={(value) =>
            onFamilyChange(value === 'v4' || value === 'v6' ? value : undefined)
          }
        >
          <SelectTrigger className='h-8 w-32' aria-label={t('history.ipFamilyColumn')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_FAMILIES}>{t('history.ipAllFamilies')}</SelectItem>
            <SelectItem value='v4'>IPv4</SelectItem>
            <SelectItem value='v6'>IPv6</SelectItem>
          </SelectContent>
        </Select>
        <IPHistoryImport />
      </div>
      <div className='overflow-hidden rounded-md border'>
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    colSpan={header.colSpan}
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
                  {family ? t('common.nothingMatches') : t('history.ipEmpty')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} totalRows={total} className='mt-auto' />
    </div>
  )
}
