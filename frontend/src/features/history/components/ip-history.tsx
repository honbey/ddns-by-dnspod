import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  listIPChanges,
  listIPHistory,
  type IPFamily,
  type IPHistoryQuery,
} from '@/features/history/api'
import { type IPChangePoint, type PagedIPChange } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ErrorState } from '@/components/error-state'
import { IPChangeChart } from '@/components/ip-change-chart'
import { IPHistoryTable } from './ip-history-table'

/** Bars the chart can show before the addresses stop being readable. */
const CHART_BARS = 12

/** Shared empty lists, so an absent answer does not rebuild the arrays. */
const NO_POINTS: IPChangePoint[] = []
const NO_CHANGES: PagedIPChange['items'] = []

/**
 * This host's public address over time, independent of any domain or task.
 *
 * Two readings of the same rows: the chart answers "how long did that address
 * last", the table answers "when did it change, and to what". They are fetched
 * separately on purpose - the chart always shows the newest changes, while the
 * table pages through the whole history, imported rows included.
 */
export function IPHistory() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [family, setFamily] = useState<IPFamily | undefined>(undefined)

  const query = useMemo<IPHistoryQuery>(
    () => ({ page, page_size: pageSize, ...(family ? { family } : {}) }),
    [page, pageSize, family]
  )
  const history = useQuery({
    queryKey: ['ip-changes', 'history', query],
    queryFn: () => listIPHistory(query),
  })

  // Not the dashboard's `['ip-changes']` key: it plots a different number of
  // bars, and sharing a key with a different limit would hand one page the
  // other's slice. The import invalidates both, because it matches the prefix.
  const changes = useQuery({
    queryKey: ['ip-changes', 'history-chart', CHART_BARS],
    queryFn: () => listIPChanges(CHART_BARS),
    staleTime: 60_000,
  })

  const items = history.data?.items ?? NO_CHANGES
  const total = history.data?.total ?? 0

  const summary = history.isPending
    ? t('history.loading')
    : total === 1
      ? t('history.ipCountOne')
      : t('history.ipCount', { total })

  return (
    <div className='flex flex-1 flex-col gap-4'>
      <Card className='flex flex-col'>
        <CardHeader>
          <CardTitle>{t('dashboard.addressLifetime')}</CardTitle>
          <CardDescription>
            {t('history.ipChartDescription', { count: CHART_BARS })}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex h-72 items-center justify-center'>
          {/* A failed refresh keeps the last chart on screen; only a failed
              first load has nothing to show. */}
          {changes.isError && !changes.data ? (
            <ErrorState onRetry={() => void changes.refetch()} />
          ) : (
            <IPChangeChart points={changes.data ?? NO_POINTS} max={CHART_BARS} />
          )}
        </CardContent>
      </Card>

      {history.isError && !history.data ? (
        <ErrorState onRetry={() => void history.refetch()} />
      ) : (
        <>
          <div className='flex items-center justify-between gap-2'>
            <p className='text-muted-foreground text-sm'>{summary}</p>
            {/* A failed refresh keeps the rows on screen; the retry sits
                beside them instead of replacing them. */}
            {history.isError ? (
              <Button variant='ghost' size='sm' onClick={() => void history.refetch()}>
                {t('common.retry')}
              </Button>
            ) : null}
          </div>
          <IPHistoryTable
            data={items}
            total={total}
            page={page}
            pageSize={pageSize}
            family={family}
            loading={history.isPending}
            onPageChange={setPage}
            onPageSizeChange={(size) => {
              setPageSize(size)
              setPage(1)
            }}
            onFamilyChange={(next) => {
              setFamily(next)
              setPage(1)
            }}
          />
        </>
      )}
    </div>
  )
}
