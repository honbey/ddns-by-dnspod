import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, Send } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import {
  createChannel,
  deleteChannel,
  listChannels,
  testChannel,
  type ChannelInput,
  updateChannel,
} from '@/features/notifications/api'
import { handleServerError } from '@/lib/handle-server-error'
import { mapBounded } from '@/lib/lists'
import { type NotifyChannel } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { ChannelDialog } from './components/channel-dialog'
import { type ChannelRowActions } from './components/channels-columns'
import { ChannelsTable } from './components/channels-table'
import { channelTypeLabel, parseConfig } from './components/channel-types'

/** How many test messages may be in flight at once; each one goes out over the network. */
const TEST_CONCURRENCY = 2

/**
 * Where a change is announced.
 *
 * Nothing here is required - a deployment with no channels simply does not
 * announce anything. The test button is the only way to know a channel works
 * before it matters.
 */
export function Notifications() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const channelsQuery = useQuery({
    queryKey: ['channels'],
    queryFn: listChannels,
  })
  const channels = channelsQuery.data ?? []

  const [editing, setEditing] = useState<NotifyChannel | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [removing, setRemoving] = useState<NotifyChannel[] | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['channels'] })

  const { mutateAsync: save, isPending: saving } = useMutation({
    mutationFn: async ({ id, input }: { id?: number; input: ChannelInput }) =>
      id ? updateChannel(id, input) : createChannel(input),
    onSuccess: async () => {
      await invalidate()
      setDialogOpen(false)
      toast.success(t('notifications.saved'))
    },
  })

  const {
    mutate: test,
    mutateAsync: testAsync,
    isPending: testing,
  } = useMutation({
    mutationFn: (id: number) => testChannel(id),
    onSuccess: () => toast.success(t('notifications.messageSent')),
    onError: (error) => handleServerError(error, t('notifications.nothingDelivered')),
  })

  // Same reasoning as on the tasks page: one mutation's isPending only follows
  // its own call, which is not what a button over a list should show.
  const [testProgress, setTestProgress] = useState<{
    done: number
    total: number
  } | null>(null)

  const testMany = async (ids: number[]) => {
    if (ids.length === 0 || testProgress) return
    setTestProgress({ done: 0, total: ids.length })
    await mapBounded(ids, TEST_CONCURRENCY, async (id) => {
      try {
        await testAsync(id)
      } catch {
        // Reported per channel by the mutation's own onError.
      } finally {
        setTestProgress((state) => (state ? { ...state, done: state.done + 1 } : state))
      }
    })
    setTestProgress(null)
  }

  const { mutateAsync: toggle } = useMutation({
    mutationFn: (channel: NotifyChannel) =>
      updateChannel(channel.id, {
        name: channel.name,
        type: channel.type,
        enabled: !channel.enabled,
        // Written back unchanged: only the flag is being changed. Parsed with
        // the tolerant reader, so a channel whose stored config is malformed can
        // still be switched off instead of failing on the way out.
        config: parseConfig(channel),
      }),
    onSuccess: async () => {
      await invalidate()
      toast.success(t('notifications.switched'))
    },
    onError: (error) => handleServerError(error, t('notifications.switchFailed')),
  })

  const { mutateAsync: remove, isPending: removingPending } = useMutation({
    mutationFn: async (selected: NotifyChannel[]) => {
      for (const channel of selected) await deleteChannel(channel.id)
    },
    onSuccess: () => toast.success(t('notifications.removed')),
    onSettled: async () => {
      // Runs on failure too; see removeSelected in records for the reasoning.
      await invalidate()
      setRemoving(null)
    },
  })

  const actions = useMemo<ChannelRowActions>(
    () => ({
      onEdit: (channel) => {
        setEditing(channel)
        setDialogOpen(true)
      },
      onTest: (channel) => test(channel.id),
      onToggle: (channel) => toggle(channel),
      onDelete: (channel) => setRemoving([channel]),
    }),
    [test, toggle]
  )

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4'>
        <PageHeading
          title={t('nav.notifications')}
          subtitle={t('notifications.subtitle')}
          actions={
            <>
              <Button
                size='sm'
                variant='outline'
                disabled={testProgress !== null || channels.length === 0}
                onClick={() => void testMany(channels.map((channel) => channel.id))}
              >
                {testProgress || testing ? (
                  <Loader2 className='animate-spin' />
                ) : (
                  <Send className='size-4' />
                )}
                {testProgress
                  ? t('notifications.testing', {
                      done: testProgress.done,
                      total: testProgress.total,
                    })
                  : t('notifications.testAll')}
              </Button>
              <Button
                size='sm'
                onClick={() => {
                  setEditing(null)
                  setDialogOpen(true)
                }}
              >
                <Plus className='size-4' />
                {t('notifications.add')}
              </Button>
            </>
          }
        />

        {channelsQuery.isError ? (
          <ErrorState onRetry={() => void channelsQuery.refetch()} />
        ) : (
          <ChannelsTable
            data={channels}
            loading={channelsQuery.isPending}
            actions={actions}
            onBulkTest={(selected) => void testMany(selected.map((channel) => channel.id))}
            onBulkSetEnabled={(selected, enabled) =>
              selected
                .filter((channel) => channel.enabled !== enabled)
                // Toasted by the mutation's own onError; caught only so the
                // rejection is not left unhandled.
                .forEach((channel) => void toggle(channel).catch(() => {}))
            }
            onBulkDelete={setRemoving}
          />
        )}
      </Main>

      <ChannelDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        channel={editing}
        saving={saving}
        onSubmit={async (input) => {
          try {
            await save({ id: editing?.id, input })
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
            ? t('notifications.removeOne', { name: removing[0]?.name ?? '' })
            : t('notifications.removeMany', { count: removing?.length ?? 0 })
        }
        desc={
          removing?.length === 1
            ? t('notifications.stopOne', {
                type: channelTypeLabel(removing[0]?.type ?? ''),
              })
            : t('notifications.stopMany')
        }
        confirmText={t('common.remove')}
        // Toasted by the global mutation handler; caught only so the rejection
        // is not left unhandled.
        handleConfirm={() => void remove(removing ?? []).catch(() => {})}
      />
    </>
  )
}
