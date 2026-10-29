import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { listHistory, type HistoryQuery } from '@/features/history/api'
import type { PagedHistoryEntry } from '@/lib/types'
import type { FacetedOption } from '@/components/data-table'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { HistoryTable } from './components/history-table'

/**
 * Enough rows to scroll through, small enough to fetch as one page.
 *
 * The window is bounded on purpose - a flapping record writes a row every few
 * minutes - so the heading says which slice of the history is on screen rather
 * than claiming to be all of it.
 */
const PAGE_SIZE = 200

type HistoryProps = {
  /** Restricted to one host's past when it arrives from a task row. */
  taskId?: number
  title?: string
  subtitle?: string
}

/**
 * Every change and failure, newest first.
 *
 * Read-only on purpose: a row is a fact about the past, and rewriting it would
 * only make it useless for working out what happened. Runs where the address
 * did not change are deliberately not here - the task's own last status covers
 * those - otherwise the table would be a flood of "IP unchanged".
 */
/**
 * One shared empty list. `data?.items ?? []` builds a new array on every render,
 * which makes anything keyed on it - the memo below, a table's row model - think
 * the data changed when nothing did.
 */
const NO_ENTRIES: PagedHistoryEntry['items'] = []

export function History({ taskId, title, subtitle }: HistoryProps) {
  const { t } = useTranslation()
  const headingTitle = title ?? t('nav.history')
  const headingNote = subtitle ?? t('history.subtitle')
  const query = useMemo<HistoryQuery>(
    () => ({ page: 1, page_size: PAGE_SIZE, ...(taskId ? { task_id: taskId } : {}) }),
    [taskId]
  )
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

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4'>
        <PageHeading
          title={headingTitle}
          subtitle={
            history.isPending
              ? t('history.loading')
              : entries.length < total
                ? t('history.newestOf', {
                    shown: entries.length,
                    total,
                    note: headingNote,
                  })
                : `${
                    total === 1 ? t('history.countOne') : t('history.count', { total })
                  } - ${headingNote}`
          }
        />

        {history.isError ? (
          <ErrorState onRetry={() => void history.refetch()} />
        ) : (
          <HistoryTable data={entries} loading={history.isPending} domainOptions={domainOptions} />
        )}
      </Main>
    </>
  )
}
