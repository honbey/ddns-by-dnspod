import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ListChecks, RefreshCw, TriangleAlert } from 'lucide-react'
import { AccountIcon, DomainIcon, IpV4Icon, IpV6Icon } from '@/components/icons/nav-icons'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { listAccounts } from '@/features/accounts/api'
import { listDomains } from '@/features/domains/api'
import { listHistory, listIPChanges } from '@/features/history/api'
import { currentIP } from '@/features/system/api'
import { listTasks, reloadScheduler } from '@/features/tasks/api'
import { describeError } from '@/lib/describe-error'
import { displayName } from '@/lib/auth'
import { greetingKey } from '@/lib/greeting'
import { type IPChangePoint } from '@/lib/types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { IPChangeChart } from '@/components/ip-change-chart'
import { AddressCard } from './components/address-card'
import { RecentChanges } from './components/recent-changes'
import { StatCard } from './components/stat-card'
import { TaskHealth } from './components/task-health'

/**
 * How many entries the list column will scroll through before the history page
 * takes over, and how far back the history query looks.
 */
const RECENT = 20
const HISTORY_WINDOW = 200

/** How many address changes the chart plots. */
const CHART_BARS = 7

/**
 * The state of this host, in the order a person looks for it: what address the
 * world sees, whether the tasks are actually keeping up with it, how big the
 * setup is, and what changed lately.
 *
 * The page needs one answer per question and shares them across the whole
 * layout, so nothing on it can disagree with anything else.
 */
export function Dashboard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)

  const tasks = useQuery({
    queryKey: ['tasks', 'dashboard'],
    queryFn: () => listTasks({ page: 1, page_size: HISTORY_WINDOW }),
  })
  // Counted by the server instead of inside the window above: that list stops at
  // two hundred, and anything past that was missing from the count.
  const failingCount = useQuery({
    queryKey: ['tasks', 'dashboard', 'failing'],
    queryFn: () => listTasks({ page: 1, page_size: 1, status: 'FAIL' }),
  })
  const domains = useQuery({ queryKey: ['domains'], queryFn: () => listDomains() })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: () => listAccounts() })
  const history = useQuery({
    queryKey: ['history', 'dashboard'],
    queryFn: () => listHistory({ page: 1, page_size: HISTORY_WINDOW }),
  })
  const changes = useQuery({
    queryKey: ['ip-changes'],
    queryFn: () => listIPChanges(CHART_BARS),
    staleTime: 60_000,
  })
  const ipv4 = useQuery({
    queryKey: ['ip', 'v4'],
    queryFn: () => currentIP('v4'),
    staleTime: 30_000,
  })
  const ipv6 = useQuery({
    queryKey: ['ip', 'v6'],
    queryFn: () => currentIP('v6'),
    staleTime: 30_000,
  })

  const reload = useMutation({
    mutationFn: reloadScheduler,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['tasks'] })
      toast.success(t('dashboard.scheduleReloaded'))
    },
  })

  const all = tasks.data?.items ?? []
  const entries = history.data?.items ?? []
  const failing = all.filter((task) => task.last_status === 'FAIL')
  const unreachable = tasks.isError || ipv4.isError

  /**
   * The change log only holds real changes, so the newest entry per family is
   * when the current address was written - the clock "unchanged for" runs from.
   */
  const sinceFor = (family: 'v4' | 'v6'): number | null => {
    const points = (changes.data ?? []).filter((point: IPChangePoint) => point.family === family)
    const last = points[points.length - 1]
    if (!last) return null
    return Math.max(0, (Date.now() - new Date(last.time).getTime()) / 1000)
  }

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4'>
        <PageHeading
          title={t('nav.dashboard')}
          subtitle={t(greetingKey(new Date().getHours()), { name: displayName(user) })}
          actions={
            <Button
              size='sm'
              variant='outline'
              disabled={reload.isPending}
              onClick={() => reload.mutate()}
            >
              <RefreshCw className={reload.isPending ? 'animate-spin' : undefined} />
              {t('dashboard.reloadSchedule')}
            </Button>
          }
        />

        {unreachable ? (
          <Alert variant='destructive'>
            <TriangleAlert />
            <AlertTitle>{t('dashboard.serverDidNotAnswer')}</AlertTitle>
            <AlertDescription>
              {describeError(tasks.error ?? ipv4.error, t('dashboard.unreachableHint'))}
            </AlertDescription>
          </Alert>
        ) : null}

        <div className='grid gap-4 lg:grid-cols-2'>
          <AddressCard
            family='v4'
            icon={IpV4Icon}
            ip={ipv4.data?.ip}
            available={ipv4.data?.available}
            message={ipv4.data?.message}
            sinceSec={sinceFor('v4')}
            loading={ipv4.isPending}
          />
          <AddressCard
            family='v6'
            icon={IpV6Icon}
            ip={ipv6.data?.ip}
            available={ipv6.data?.available}
            message={ipv6.data?.message}
            sinceSec={sinceFor('v6')}
            loading={ipv6.isPending}
          />
        </div>

        <TaskHealth tasks={all} failing={failing} loading={tasks.isPending} />

        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          <StatCard
            title={t('dashboard.statTasks')}
            // The query reads one window, so the number of tasks is the total
            // the server reports, not the length of that window.
            value={tasks.isPending ? '-' : (tasks.data?.total ?? 0)}
            icon={ListChecks}
            loading={tasks.isPending}
          />
          <StatCard
            title={t('dashboard.statFailing')}
            value={failingCount.isPending ? '-' : (failingCount.data?.total ?? 0)}
            icon={TriangleAlert}
            loading={failingCount.isPending}
          />
          <StatCard
            title={t('dashboard.statDomains')}
            value={domains.isPending ? '-' : (domains.data?.length ?? 0)}
            icon={DomainIcon}
            loading={domains.isPending}
          />
          <StatCard
            title={t('dashboard.statAccounts')}
            value={accounts.isPending ? '-' : (accounts.data?.length ?? 0)}
            icon={AccountIcon}
            loading={accounts.isPending}
          />
        </div>

        <div className='grid gap-4 lg:grid-cols-7'>
          <Card className='flex h-full flex-col lg:col-span-4'>
            <CardHeader>
              <CardTitle>{t('dashboard.addressLifetime')}</CardTitle>
              <CardDescription>
                {t('dashboard.addressLifetimeDescription', { count: CHART_BARS })}
              </CardDescription>
            </CardHeader>
            <CardContent className='min-h-0 flex-1'>
              <IPChangeChart points={changes.data ?? []} max={CHART_BARS} />
            </CardContent>
          </Card>

          <div className='lg:col-span-3'>
            <RecentChanges
              entries={entries.slice(0, RECENT)}
              loading={history.isPending}
              total={history.data?.total ?? 0}
            />
          </div>
        </div>
      </Main>
    </>
  )
}
