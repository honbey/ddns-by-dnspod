import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { listHistory, type HistoryQuery } from '@/features/history/api'
import type { PagedHistoryEntry } from '@/lib/types'
import type { FacetedOption } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { HistoryTable } from './history-table'

/**
 * Enough rows to scroll through, small enough to fetch as one page.
 *
 * The window is bounded on purpose - a flapping record writes a row every few
 * minutes - so the line above the table says which slice is on screen rather
 * than claiming to be all of it.
 */
const PAGE_SIZE = 200

/**
 * One shared empty list. `data?.items ?? []` builds a new array on every
 * render, which makes anything keyed on it - a table's row model included -
 * think the data changed when nothing did.
 */
const NO_ENTRIES: PagedHistoryEntry['items'] = []

/**
 * Every change and failure a task has recorded, newest first.
 *
 * Read-only on purpose: a row is a fact about the past, and rewriting it would
 * only make it useless for working out what happened. Runs where the address
 * did not change are deliberately not here - the task's own last status covers
 * those - otherwise the table would be a flood of "IP unchanged".
 */
export function TaskHistory() {
  const { t } = useTranslation()
  const query = useMemo<HistoryQuery>(() => ({ page: 1, page_size: PAGE_SIZE }), [])
  const history = useQuery({
    queryKey: ['history', query],
    queryFn: () => listHistory(query),
  })

  const entries = history.data?.items ?? NO_ENTRIES
  const total = history.data?.total ?? 0
  const domainOptions = useMemo<FacetedOption[]>(
    () =>
      [...new Set(entries.map((entry) => entry.domain))]
        .sort()
        .map((domain) => ({ label: domain, value: domain })),
    [entries]
  )

  const summary = history.isPending
    ? t('history.loading')
    : entries.length < total
      ? t('history.newestOf', {
          shown: entries.length,
          total,
          note: t('history.subtitle'),
        })
      : `${total === 1 ? t('history.countOne') : t('history.count', { total })} - ${t('history.subtitle')}`

  return (
    <div className='flex flex-1 flex-col gap-4'>
      <p className='text-muted-foreground text-sm'>{summary}</p>
      {history.isError ? (
        <ErrorState onRetry={() => void history.refetch()} />
      ) : (
        <HistoryTable data={entries} loading={history.isPending} domainOptions={domainOptions} />
      )}
    </div>
  )
}
