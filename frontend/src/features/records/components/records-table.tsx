import { useState } from 'react'
import {
  type SortingState,
  type Table,
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
import { Trash2 } from 'lucide-react'
import { type DnsRecord, type Group } from '@/lib/types'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table as TableComponent,
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
import { useRecordsColumns, type RecordRowActions } from './records-columns'

/**
 * The stable record types, offered as filters. A hand-written list because the
 * values come back per row, and a faceted list of a thousand rows would offer
 * duplicates.
 */
const TYPE_OPTIONS = [
  { label: 'A', value: 'A' },
  { label: 'AAAA', value: 'AAAA' },
  { label: 'CNAME', value: 'CNAME' },
  { label: 'MX', value: 'MX' },
  { label: 'TXT', value: 'TXT' },
]

type RecordsTableProps = {
  data: DnsRecord[]
  loading: boolean
  /** The domain's record groups, for the filter above the table. */
  groups: Group[]
  /** 0 shows every group. */
  groupId: number
  onGroupChange: (groupId: number) => void
  actions: RecordRowActions
  onBulkDelete: (records: DnsRecord[]) => void
}

/** A table shaped like the file browser's, so both read the same way. */
export function TableShell({
  table,
  children,
}: {
  table: Table<DnsRecord>
  children: React.ReactNode
}) {
  return (
    <div className='overflow-hidden rounded-md border'>
      <TableComponent>
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
        <TableBody>{children}</TableBody>
      </TableComponent>
    </div>
  )
}

const ALL_GROUPS = '0'

export function RecordsTable({
  data,
  loading,
  groups,
  groupId,
  onGroupChange,
  actions,
  onBulkDelete,
}: RecordsTableProps) {
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([{ id: 'sub_domain', desc: false }])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [globalFilter, setGlobalFilter] = useState('')

  const { t } = useTranslation()
  const columns = useRecordsColumns(actions)
  const statusOptions = [
    { label: t('status.enabled'), value: 'ENABLE' },
    { label: t('status.disabled'), value: 'DISABLE' },
  ]

  const table = useReactTable({
    data,
    columns,
    // Keyed by the record id rather than the row index, so a refetch after a
    // delete cannot leave the selection pointing at a different record.
    getRowId: (row) => String(row.record_id),
    state: { sorting, columnVisibility, globalFilter, rowSelection },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = String(filterValue).toLowerCase()
      const record = row.original
      return [record.sub_domain, record.record_type, record.value, record.line]
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

  const selectedRows = table.getFilteredSelectedRowModel().rows

  return (
    <div className={cn('max-sm:has-[div[role="toolbar"]]:mb-16', 'flex flex-1 flex-col gap-4')}>
      <DataTableToolbar
        table={table}
        searchPlaceholder={t('table.filter')}
        filters={[
          { columnId: 'record_type', title: t('ui.type'), options: TYPE_OPTIONS },
          {
            columnId: 'status',
            title: t('ui.status'),
            options: statusOptions,
          },
        ]}
      >
        {/* A request parameter rather than a column filter: the listing cannot
            say which group a record is in, so only one group can be asked for
            at a time. */}
        <Select
          value={String(groupId)}
          onValueChange={(value) => onGroupChange(Number(value))}
          disabled={groups.length === 0}
        >
          <SelectTrigger className='h-8 w-40' aria-label={t('records.groupLabel')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_GROUPS}>{t('records.groupAll')}</SelectItem>
            {groups.map((group) => (
              <SelectItem key={group.id} value={String(group.id)}>
                {group.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </DataTableToolbar>
      <TableShell table={table}>
        {loading ? (
          Array.from({ length: 5 }).map((_, rowIndex) => (
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
            <TableRow key={row.id} data-state={row.getIsSelected() && 'selected'}>
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
            <TableCell colSpan={columns.length} className='text-muted-foreground h-24 text-center'>
              {data.length === 0 ? t('records.empty') : t('common.nothingMatches')}
            </TableCell>
          </TableRow>
        )}
      </TableShell>
      <DataTablePagination table={table} className='mt-auto' />
      <DataTableBulkActions table={table} entityName={t('entity.record')}>
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
