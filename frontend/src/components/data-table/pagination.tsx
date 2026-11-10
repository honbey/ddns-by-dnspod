import {
  ChevronLeftIcon,
  ChevronRightIcon,
  DoubleArrowLeftIcon,
  DoubleArrowRightIcon,
} from '@radix-ui/react-icons'
import { type Table } from '@tanstack/react-table'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn, isNavigationTarget, parsePageTarget } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

type DataTablePaginationProps<TData> = {
  table: Table<TData>
  /**
   * The row count the API reports, needed by a server-side table. Client-side
   * tables leave it out: they paginate the rows they hold, so the row count and
   * the page count have to come from that same set.
   */
  totalRows?: number
  className?: string
}

export function DataTablePagination<TData>({
  table,
  totalRows,
  className,
}: DataTablePaginationProps<TData>) {
  const { t } = useTranslation()
  const currentPage = table.getState().pagination.pageIndex + 1
  const totalPages = Math.max(1, table.getPageCount())
  const viewRows = table.getPrePaginationRowModel().rows.length
  // A server-side table holds one page at a time, so only the total it was given
  // describes the list. A client-side table holds what it loaded, and filtering
  // shrinks that set - the page count follows it, so the row count must too.
  const rowCount = table.options.manualPagination === true ? (totalRows ?? viewRows) : viewRows
  const pageLabel =
    rowCount === 1 ? t('table.totalRowsOne') : t('table.totalRows', { total: rowCount })

  // The input follows the table until someone types in it.
  const [pageInput, setPageInput] = useState(String(currentPage))
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    setPageInput(String(currentPage))
    setDirty(false)
  }, [currentPage])

  const commitPage = () => {
    const target = parsePageTarget(pageInput, currentPage, totalPages)
    table.setPageIndex(target - 1)
    setPageInput(String(target))
    setDirty(false)
  }

  const pageDescriptionId = useId()
  const navigationRef = useRef<HTMLDivElement>(null)

  return (
    <div
      className={cn('flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-2', className)}
    >
      <p className='text-sm font-medium text-muted-foreground'>{pageLabel}</p>

      <div className='flex flex-wrap items-center gap-2'>
        <Select
          value={`${table.getState().pagination.pageSize}`}
          onValueChange={(value) => {
            table.setPageSize(Number(value))
          }}
        >
          {/* The control needs a name of its own: the text beside it is not
              associated with the trigger. */}
          <SelectTrigger className='h-8 w-17.5' aria-label={t('table.rowsPerPage')}>
            <SelectValue placeholder={table.getState().pagination.pageSize} />
          </SelectTrigger>
          <SelectContent side='top'>
            {[10, 20, 30, 40, 50].map((pageSize) => (
              <SelectItem key={pageSize} value={`${pageSize}`}>
                {pageSize}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className='text-sm font-medium text-muted-foreground'>{t('table.perPage')}</span>

        <div ref={navigationRef} className='flex items-center gap-1 sm:ms-2'>
          <Button
            variant='outline'
            className='size-8 p-0'
            onClick={() => table.setPageIndex(0)}
            disabled={!table.getCanPreviousPage()}
          >
            <span className='sr-only'>{t('table.goToFirst')}</span>
            <DoubleArrowLeftIcon className='h-4 w-4' />
          </Button>
          <Button
            variant='outline'
            className='size-8 p-0'
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            <span className='sr-only'>{t('table.goToPrevious')}</span>
            <ChevronLeftIcon className='h-4 w-4' />
          </Button>

          <Input
            type='text'
            inputMode='numeric'
            value={pageInput}
            onChange={(event) => {
              setPageInput(event.target.value.replace(/\D/g, ''))
              setDirty(true)
            }}
            onBlur={(event) => {
              // A navigation button blurs the input first; committing here as
              // well would navigate twice for one interaction.
              if (isNavigationTarget(event.relatedTarget, navigationRef.current)) {
                setDirty(false)
                setPageInput(String(currentPage))
                return
              }
              if (dirty) commitPage()
            }}
            // Enter commits but keeps focus: blurring here would run the blur
            // handler before state updates, committing the same page twice.
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                commitPage()
              }
            }}
            disabled={totalPages <= 1}
            aria-label={t('table.pageNumberLabel')}
            aria-describedby={pageDescriptionId}
            className='h-8 w-12 px-0 text-center tabular-nums'
          />
          {/* The current page is a description of the input, not its name: the
              name has to stay put while the page changes. */}
          <span id={pageDescriptionId} className='sr-only'>
            {t('table.pageOf', { page: currentPage, total: totalPages })}
          </span>
          <span
            aria-hidden='true'
            className='text-sm font-medium text-muted-foreground tabular-nums'
          >
            / {totalPages}
          </span>

          <Button
            variant='outline'
            className='size-8 p-0'
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            <span className='sr-only'>{t('table.goToNext')}</span>
            <ChevronRightIcon className='h-4 w-4' />
          </Button>
          <Button
            variant='outline'
            className='size-8 p-0'
            onClick={() => table.setPageIndex(totalPages - 1)}
            disabled={!table.getCanNextPage()}
          >
            <span className='sr-only'>{t('table.goToLast')}</span>
            <DoubleArrowRightIcon className='h-4 w-4' />
          </Button>
        </div>
      </div>
    </div>
  )
}
