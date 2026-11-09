import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Play } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { listAccounts } from '@/features/accounts/api'
import {
  createTask,
  deleteTask,
  listAllTasks,
  runTask,
  type TaskInput,
  updateTask,
} from '@/features/tasks/api'
import { handleServerError } from '@/lib/handle-server-error'
import { mapBounded } from '@/lib/lists'
import { type DDNSTask } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { ConfigDrawer } from '@/components/config-drawer'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { TaskDialog } from './components/task-dialog'
import { hostOf, type TaskRowActions } from './components/tasks-columns'
import { TasksTable } from './components/tasks-table'

/** Everything down to the account id is what the API takes; the rest is history. */
function asInput(task: DDNSTask): TaskInput {
  return {
    account_id: task.account_id,
    domain: task.domain,
    sub_domain: task.sub_domain,
    record_type: task.record_type,
    record_line: task.record_line,
    ttl: task.ttl,
    cron_expr: task.cron_expr,
    enabled: task.enabled,
    notify_on_change: task.notify_on_change,
  }
}

/**
 * How many requests a bulk run keeps in flight.
 *
 * Every one of them ends in a DNSPod write, so the pool is what keeps "Run all"
 * from arriving as a burst: small enough to stay clear of the provider's rate
 * limit, wide enough that a long list still finishes in one sitting.
 */
const RUN_CONCURRENCY = 4

/**
 * The list of hosts this deployment keeps pointed at itself.
 *
 * Nothing here touches DNS directly: a change is saved to the store and the
 * schedule is rebuilt by the server, which is why "reload" never appears - it is
 * part of the write.
 */
export function Tasks() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  // Every task, not the first page: the table filters and paginates client
  // side, and "Run all" can only act on what the page actually holds.
  const tasksQuery = useQuery({
    queryKey: ['tasks', 'all'],
    queryFn: () => listAllTasks(),
  })
  const accountsQuery = useQuery({
    queryKey: ['accounts'],
    queryFn: () => listAccounts(),
  })

  const tasks = tasksQuery.data?.items ?? []
  const accountNames = useMemo(
    () => new Map((accountsQuery.data ?? []).map((account) => [account.id, account.name])),
    [accountsQuery.data]
  )

  const [editing, setEditing] = useState<DDNSTask | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [removing, setRemoving] = useState<DDNSTask[] | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['tasks'] })

  const { mutateAsync: save, isPending: saving } = useMutation({
    mutationFn: async ({ id, input }: { id?: number; input: TaskInput }) =>
      id ? updateTask(id, input) : createTask(input),
    onSuccess: async (task) => {
      await invalidate()
      setDialogOpen(false)
      toast.success(t('tasks.saved', { host: `${task.sub_domain || '@'}.${task.domain}` }))
    },
  })

  const { mutateAsync: toggle } = useMutation({
    mutationFn: (task: DDNSTask) =>
      updateTask(task.id, { ...asInput(task), enabled: !task.enabled }),
    onSuccess: async (task) => {
      await invalidate()
      toast.success(
        task.enabled
          ? `${task.sub_domain || '@'}.${task.domain} resumed`
          : `${task.sub_domain || '@'}.${task.domain} paused`
      )
    },
  })

  const { mutate: run, mutateAsync: runAsync } = useMutation({
    mutationFn: (id: number) => runTask(id),
    onSuccess: async (result) => {
      await Promise.all([invalidate(), queryClient.invalidateQueries({ queryKey: ['history'] })])
      if (result.status === 'FAIL') {
        toast.error(`${result.sub_domain || '@'}.${result.domain}: ${result.message || 'failed'}`)
        return
      }
      toast.success(
        result.changed
          ? `${result.sub_domain || '@'}.${result.domain} now points at ${result.new_ip}`
          : `${result.sub_domain || '@'}.${result.domain} was already right`
      )
    },
    // A manual run that fails on the wire has no other reporter: Run all
    // triggers one mutation per task, and without this the failures are silent.
    onError: (error, id) => {
      const task = tasks.find((item) => item.id === id)
      handleServerError(
        error,
        task ? t('tasks.runFailedOne', { host: hostOf(task) }) : t('tasks.runFailedMany', { id })
      )
    },
  })

  // How far a bulk run has got. A mutation's own isPending follows one call -
  // the last one to start - so a button bound to it re-enables while most of
  // the pool is still running.
  const [runProgress, setRunProgress] = useState<{
    done: number
    total: number
  } | null>(null)

  const runMany = async (ids: number[]) => {
    if (ids.length === 0 || runProgress) return
    setRunProgress({ done: 0, total: ids.length })
    await mapBounded(ids, RUN_CONCURRENCY, async (id) => {
      try {
        await runAsync(id)
      } catch {
        // Reported by the mutation's own onError; caught here so one failed
        // task does not drain the pool and strand the rest.
      } finally {
        setRunProgress((state) => (state ? { ...state, done: state.done + 1 } : state))
      }
    })
    setRunProgress(null)
  }

  const { mutateAsync: remove, isPending: removingPending } = useMutation({
    mutationFn: async (ids: number[]) => {
      for (const id of ids) await deleteTask(id)
    },
    onSuccess: () => toast.success(t('tasks.removed')),
    onSettled: async () => {
      // Runs on failure too; see removeSelected in records for the reasoning.
      await invalidate()
      setRemoving(null)
    },
  })

  const actions = useMemo<TaskRowActions>(
    () => ({
      onEdit: (task) => {
        setEditing(task)
        setDialogOpen(true)
      },
      onRun: (task) => run(task.id),
      onToggle: (task) => toggle(task),
      onDelete: (task) => setRemoving([task]),
    }),
    [run, toggle]
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
          title={t('nav.tasks')}
          subtitle={t('tasks.subtitle')}
          actions={
            <>
              <Button
                size='sm'
                variant='outline'
                disabled={runProgress !== null || tasks.length === 0}
                onClick={() => void runMany(tasks.map((task) => task.id))}
              >
                <Play className='size-4' />
                {runProgress
                  ? t('tasks.running', { done: runProgress.done, total: runProgress.total })
                  : t('tasks.runAll')}
              </Button>
              <Button
                size='sm'
                onClick={() => {
                  setEditing(null)
                  setDialogOpen(true)
                }}
              >
                <Plus className='size-4' />
                {t('tasks.add')}
              </Button>
            </>
          }
        />

        {tasksQuery.isError ? (
          <ErrorState onRetry={() => void tasksQuery.refetch()} />
        ) : (
          <TasksTable
            data={tasks}
            loading={tasksQuery.isPending}
            accountNames={accountNames}
            actions={actions}
            onBulkRun={(selected) => void runMany(selected.map((task) => task.id))}
            onBulkSetEnabled={(selected, enabled) =>
              selected
                .filter((task) => task.enabled !== enabled)
                // Toasted by the mutation's own onError; caught only so the
                // rejection is not left unhandled.
                .forEach((task) => void toggle(task).catch(() => {}))
            }
            onBulkDelete={setRemoving}
          />
        )}
      </Main>

      <TaskDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        task={editing}
        saving={saving}
        onSubmit={async (input) => {
          try {
            const id = editing?.id
            await save({ id, input })
          } catch {
            // Swallowed: the global mutation handler has already reported the
            // failure, with the server's own wording, and the dialog is left
            // open so what was typed is still there to retry with.
          }
        }}
      />

      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) setRemoving(null)
        }}
        destructive
        isLoading={removingPending}
        title={
          removing?.length === 1
            ? t('tasks.stopWatching', {
                host: `${removing[0]?.sub_domain || '@'}.${removing[0]?.domain}`,
              })
            : t('tasks.removeMany', { count: removing?.length ?? 0 })
        }
        desc={t('tasks.removeDescription')}
        confirmText={t('common.remove')}
        // Toasted by the global mutation handler; caught only so the rejection
        // is not left unhandled.
        handleConfirm={() => void remove((removing ?? []).map((task) => task.id)).catch(() => {})}
      />
    </>
  )
}
