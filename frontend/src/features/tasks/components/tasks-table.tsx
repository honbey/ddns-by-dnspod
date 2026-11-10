import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Pause, Play, Trash2 } from 'lucide-react'
import { type DDNSTask } from '@/lib/types'
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
import { useTasksColumns, type TaskRowActions } from './tasks-columns'

const typeOptions = [
  { label: 'A', value: 'A' },
  { label: 'AAAA', value: 'AAAA' },
]

type TasksTableProps = {
  data: DDNSTask[]
  loading: boolean
  accountNames: Map<number, string>
  actions: TaskRowActions
  onBulkRun: (tasks: DDNSTask[]) => void
  onBulkSetEnabled: (tasks: DDNSTask[], enabled: boolean) => void
  onBulkDelete: (tasks: DDNSTask[]) => void
}

/** Every host this deployment keeps up to date. */
export function TasksTable({
  data,
  loading,
  accountNames,
  actions,
  onBulkRun,
  onBulkSetEnabled,
  onBulkDelete,
}: TasksTableProps) {
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([{ id: 'host', desc: false }])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [globalFilter, setGlobalFilter] = useState('')

  const { t } = useTranslation()
  const columns = useTasksColumns(actions, accountNames)
  const stateOptions = [
    { label: t('status.enabled'), value: 'enabled' },
    { label: t('tasks.paused'), value: 'paused' },
  ]

  const table = useReactTable({
    data,
    columns,
    // Keyed by the task id rather than the row index, so a refetch after a
    // delete cannot leave the selection pointing at a different task.
    getRowId: (row) => String(row.id),
    state: { sorting, columnVisibility, globalFilter, rowSelection },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = String(filterValue).toLowerCase()
      const task = row.original
      return (
        `${task.sub_domain}.${task.domain}`.toLowerCase().includes(needle) ||
        task.domain.toLowerCase().includes(needle) ||
        task.current_ip.includes(needle) ||
        task.cron_expr.toLowerCase().includes(needle) ||
        (task.last_message ?? '').toLowerCase().includes(needle)
      )
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
          { columnId: 'record_type', title: t('ui.type'), options: typeOptions },
          { columnId: 'enabled', title: t('tasks.stateColumn'), options: stateOptions },
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
              Array.from({ length: 4 }).map((_, rowIndex) => (
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
                  {data.length === 0 ? t('tasks.empty') : t('common.nothingMatches')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} className='mt-auto' />
      <DataTableBulkActions table={table} entityName={t('entity.task')}>
        <Button
          variant='outline'
          size='sm'
          onClick={() => onBulkRun(selectedRows.map((row) => row.original))}
        >
          <Play />
          {t('common.run')}
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
          <Pause />
          {t('common.pause')}
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
          <Play />
          {t('common.resume')}
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
