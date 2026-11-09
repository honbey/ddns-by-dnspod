import { Cross2Icon } from '@radix-ui/react-icons'
import { useId } from 'react'
import { type Table } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DataTableFacetedFilter, type FacetedOption } from './faceted-filter'
import { DataTableViewOptions } from './view-options'

type DataTableToolbarProps<TData> = {
  table: Table<TData>
  searchPlaceholder?: string
  searchKey?: string
  /** Faceted filters, one per column, beside the search box. */
  filters?: {
    columnId: string
    title: string
    /** Omitted when a column's values are not a fixed set; see the filter. */
    options?: FacetedOption[]
  }[]
}

export function DataTableToolbar<TData>({
  table,
  searchPlaceholder,
  searchKey,
  filters = [],
}: DataTableToolbarProps<TData>) {
  const { t } = useTranslation()
  // The filter is a lone input whose label is only a placeholder; the id is
  // what lets a browser and a screen reader treat it as a field.
  const filterId = useId()

  const isFiltered = table.getState().columnFilters.length > 0 || table.getState().globalFilter

  return (
    <div className='flex items-center justify-between'>
      <div className='flex flex-1 flex-col-reverse items-start gap-y-2 sm:flex-row sm:items-center sm:space-x-2'>
        {searchKey ? (
          <Input
            id={filterId}
            placeholder={searchPlaceholder ?? t('table.filter')}
            aria-label={searchPlaceholder ?? t('table.filter')}
            value={(table.getColumn(searchKey)?.getFilterValue() as string) ?? ''}
            onChange={(event) => table.getColumn(searchKey)?.setFilterValue(event.target.value)}
            className='h-8 w-37.5 lg:w-62.5'
          />
        ) : (
          <Input
            id={filterId}
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            value={table.getState().globalFilter ?? ''}
            onChange={(event) => table.setGlobalFilter(event.target.value)}
            className='h-8 w-37.5 lg:w-62.5'
          />
        )}
        {filters.map((filter) => (
          <DataTableFacetedFilter
            key={filter.columnId}
            column={table.getColumn(filter.columnId)}
            title={filter.title}
            options={filter.options}
          />
        ))}
        {isFiltered && (
          <Button
            variant='ghost'
            onClick={() => {
              table.resetColumnFilters()
              table.setGlobalFilter('')
            }}
            className='h-8 px-2 lg:px-3'
          >
            {t('table.resetFilters')}
            <Cross2Icon className='ms-2 h-4 w-4' />
          </Button>
        )}
      </div>
      <DataTableViewOptions table={table} />
    </div>
  )
}
