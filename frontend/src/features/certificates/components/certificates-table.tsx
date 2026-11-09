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
import { type Certificate } from '@/lib/types'
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
import { useCertificatesColumns, type CertificateRowActions } from './certificates-columns'

type CertificatesTableProps = {
  data: Certificate[]
  loading: boolean
  actions: CertificateRowActions
  accountNames: Map<number, string>
  /** False while this deployment has issuance switched off. */
  canIssue: boolean
}

/** The certificates this deployment issues and keeps renewed. */
export function CertificatesTable({
  data,
  loading,
  actions,
  accountNames,
  canIssue,
}: CertificatesTableProps) {
  const [sorting, setSorting] = useState<SortingState>([{ id: 'name', desc: false }])
  // The key type is worth seeing when it matters and worth hiding otherwise:
  // it is the one column that never answers a question about a running system.
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({
    key_type: false,
  })
  const [globalFilter, setGlobalFilter] = useState('')

  const { t } = useTranslation()
  const columns = useCertificatesColumns(actions, accountNames, canIssue)
  // RUNNING is the one worth filtering for: it answers "is something being
  // issued right now", which is what a page left open is waiting on.
  const resultOptions: FacetedOption[] = [
    { label: 'OK', value: 'OK' },
    { label: 'FAIL', value: 'FAIL' },
    { label: t('certificates.running'), value: 'RUNNING' },
  ]
  const renewalOptions: FacetedOption[] = [
    { label: t('certificates.automatic'), value: 'automatic' },
    { label: t('certificates.manual'), value: 'manual' },
  ]

  const table = useReactTable({
    data,
    columns,
    // Keyed by the certificate id rather than the row index, so a refetch -
    // which happens every two seconds while something is being issued - does
    // not re-key every row.
    getRowId: (row) => String(row.id),
    state: { sorting, columnVisibility, globalFilter },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = String(filterValue).toLowerCase()
      const cert = row.original
      return [cert.name, ...cert.domains, cert.output_path, cert.last_message]
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
          { columnId: 'last_status', title: 'Last run', options: resultOptions },
          {
            columnId: 'auto_renew',
            title: t('certificates.renewLabel'),
            options: renewalOptions,
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
                  {data.length === 0 ? t('certificates.empty') : t('common.nothingMatches')}
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
