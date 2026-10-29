import * as React from 'react'
import { type Column } from '@tanstack/react-table'
import { Check, CirclePlus, FileText, Files, Folder } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'

export type FacetedOption = {
  label: string
  value: string
  icon?: React.ComponentType<{ className?: string }>
}

/**
 * The values a column actually holds, most common first, for the case where they
 * are not a fixed set. Folders and files keep their own icons so the menu reads
 * like the column it filters.
 */
function optionsFrom(facets: Map<unknown, number> | undefined): FacetedOption[] {
  if (!facets) return []

  return [...facets.entries()]
    .filter(([value]) => typeof value === 'string')
    .sort((left, right) => right[1] - left[1] || String(left[0]).localeCompare(String(right[0])))
    .map(([value]) => ({
      label: String(value),
      value: String(value),
      icon: value === 'Folder' ? Folder : value === 'File' ? FileText : Files,
    }))
}

type DataTableFacetedFilterProps<TData, TValue> = {
  column?: Column<TData, TValue>
  title?: string
  /**
   * The values to offer. Left out when they are not a fixed set - file
   * extensions, for one - and the menu is then built from what the column
   * actually holds, most common first.
   */
  options?: FacetedOption[]
}

/**
 * A column's values as a multi-select: the menu lists what is actually in the
 * table, how many rows each value has, and what is currently selected.
 */
export function DataTableFacetedFilter<TData, TValue>({
  column,
  title,
  options,
}: DataTableFacetedFilterProps<TData, TValue>) {
  const { t } = useTranslation()
  const facets = column?.getFacetedUniqueValues()
  const selectedValues = new Set(column?.getFilterValue() as string[])
  const offered = options ?? optionsFrom(facets)

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant='outline' size='sm' className='h-8 border-dashed'>
          <CirclePlus className='size-4' />
          {title}
          {selectedValues?.size > 0 && (
            <>
              <Separator orientation='vertical' className='mx-2 h-4' />
              <Badge variant='secondary' className='rounded-sm px-1 font-normal lg:hidden'>
                {selectedValues.size}
              </Badge>
              <div className='hidden gap-1 lg:flex'>
                {selectedValues.size > 2 ? (
                  <Badge variant='secondary' className='rounded-sm px-1 font-normal'>
                    {selectedValues.size} selected
                  </Badge>
                ) : (
                  offered
                    .filter((option) => selectedValues.has(option.value))
                    .map((option) => (
                      <Badge
                        variant='secondary'
                        key={option.value}
                        className='rounded-sm px-1 font-normal'
                      >
                        {option.label}
                      </Badge>
                    ))
                )}
              </div>
            </>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent className='w-50 p-0' align='start'>
        <Command>
          {/* A placeholder is not a name: it disappears on the first keystroke */}
          <CommandInput placeholder={title} aria-label={`Filter by ${title}`} />
          <CommandList>
            <CommandEmpty>{t('nav.noResults')}</CommandEmpty>
            <CommandGroup>
              {offered.map((option) => {
                const isSelected = selectedValues.has(option.value)
                return (
                  <CommandItem
                    key={option.value}
                    onSelect={() => {
                      if (isSelected) selectedValues.delete(option.value)
                      else selectedValues.add(option.value)

                      const filterValues = Array.from(selectedValues)
                      column?.setFilterValue(filterValues.length ? filterValues : undefined)
                    }}
                  >
                    <div
                      className={cn(
                        'flex size-4 items-center justify-center rounded-sm border border-primary',
                        isSelected
                          ? 'bg-primary text-primary-foreground'
                          : 'opacity-50 [&_svg]:invisible'
                      )}
                    >
                      <Check className='size-4 text-background' />
                    </div>
                    {option.icon && <option.icon className='size-4 text-muted-foreground' />}
                    <span>{option.label}</span>
                    {facets?.get(option.value) ? (
                      <span className='ms-auto flex h-4 w-4 items-center justify-center font-mono text-xs'>
                        {facets.get(option.value)}
                      </span>
                    ) : null}
                  </CommandItem>
                )
              })}
            </CommandGroup>
            {selectedValues.size > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    onSelect={() => column?.setFilterValue(undefined)}
                    className='justify-center text-center'
                  >
                    {t('table.clearFilters')}
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
