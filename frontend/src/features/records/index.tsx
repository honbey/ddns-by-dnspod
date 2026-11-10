import { useMemo, useState } from 'react'
import { Link, useParams, useSearch } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Layers, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import {
  batchCreateRecords,
  batchDeleteRecords,
  createRecord,
  deleteRecord,
  listAllRecords,
  setRecordStatus,
  type RecordInput,
  updateRecord,
} from '@/features/records/api'
import { handleServerError } from '@/lib/handle-server-error'
import { fitsOneBatch, MAX_BATCH_ITEMS } from '@/lib/limits'
import { type DnsRecord } from '@/lib/types'
import { Button } from '@/components/ui/button'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { BatchDialog, RecordDialog } from './components/record-dialog'
import { type RecordRowActions } from './components/records-columns'
import { RecordsTable } from './components/records-table'

/**
 * The records of one domain.
 *
 * Everything here reaches DNSPod the moment it is saved, which is why the
 * destructive actions ask first.
 */
export function Records() {
  const { t } = useTranslation()
  const { domain } = useParams({ from: '/_authenticated/domains/$domain' })
  // Missing means the default account, which is what the API answers with.
  const { account_id: accountId } = useSearch({
    from: '/_authenticated/domains/$domain',
  })
  const queryClient = useQueryClient()
  // The whole listing at once: the table filters and paginates client side, so
  // a round trip per filter change would only add latency - but the first page
  // alone is not the listing, and everything past it used to be unreachable.
  const recordsQuery = useQuery({
    queryKey: ['records', domain, accountId],
    queryFn: () => listAllRecords(domain, accountId),
  })

  const [editing, setEditing] = useState<DnsRecord | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)
  const [removing, setRemoving] = useState<DnsRecord[] | null>(null)

  const records = recordsQuery.data?.items ?? []
  const total = recordsQuery.data?.total ?? 0

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['records', domain] }),
      queryClient.invalidateQueries({ queryKey: ['domains'] }),
    ])
  }

  const { mutateAsync: save, isPending: saving } = useMutation({
    mutationFn: async (input: RecordInput) =>
      editing
        ? updateRecord(domain, editing.record_id, input, accountId)
        : createRecord(domain, input, accountId),
    onSuccess: async () => {
      await invalidate()
      setDialogOpen(false)
      toast.success(t('records.saved'))
    },
  })

  const { mutate: toggle } = useMutation({
    mutationFn: (record: DnsRecord) =>
      setRecordStatus(domain, record.record_id, record.status !== 'ENABLE', accountId),
    onSuccess: async () => {
      await invalidate()
      toast.success(t('records.switched'))
    },
  })

  const { mutateAsync: removeSelected, isPending: removingPending } = useMutation({
    mutationFn: async (selected: DnsRecord[]) => {
      if (selected.length === 1) {
        await deleteRecord(domain, selected[0].record_id, accountId)
        return { deleted: 1, failed: 0 }
      }
      // One call rather than N, so a slip cannot leave half of them gone.
      const result = await batchDeleteRecords(
        domain,
        selected.map((record) => record.record_id),
        accountId
      )
      // A partial failure is a 200, not an error, so no handler anywhere sees
      // it: the count is the only thing that can reach the user, and a batch
      // that lost three of five did not "remove" them.
      return { deleted: result.deleted, failed: result.failed?.length ?? 0 }
    },
    onSuccess: ({ deleted, failed }) =>
      toast.success(
        failed ? t('records.removedPartial', { deleted, failed }) : t('records.removed')
      ),
    onSettled: async () => {
      // Runs on failure too: a failure after part of the work has gone through
      // still leaves the list stale, and a dialog that only closes on success
      // would sit open over a table the server no longer agrees with.
      await invalidate()
      setRemoving(null)
    },
  })

  const { mutate: createMany, isPending: creatingMany } = useMutation({
    mutationFn: (items: RecordInput[]) => batchCreateRecords(domain, items, accountId),
    onSuccess: async (result) => {
      await invalidate()
      setBatchOpen(false)
      const done = result.succeeded?.length ?? 0
      const failed = result.failed?.length ?? 0
      toast.success(
        failed
          ? `${done} created, ${failed} failed`
          : `${done} record${done === 1 ? '' : 's'} created`
      )
    },
    onError: (error) => handleServerError(error, t('records.createFailed')),
  })

  const actions = useMemo<RecordRowActions>(
    () => ({
      onEdit: (record) => {
        setEditing(record)
        setDialogOpen(true)
      },
      onToggle: (record) => toggle(record),
      onDelete: (record) => setRemoving([record]),
    }),
    [toggle]
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
          title={domain}
          subtitle={
            // One line under the title, as the file browser has it: where this
            // page sits, and how much the domain is holding.
            <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
              <Breadcrumb>
                <BreadcrumbList>
                  <BreadcrumbItem>
                    <BreadcrumbLink asChild>
                      <Link to='/domains'>{t('records.breadcrumbDomains')}</Link>
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbPage>{domain}</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <span className='text-xs text-muted-foreground'>
                {records.length < total
                  ? t('records.showingFirst', { shown: records.length, total })
                  : total === 1
                    ? t('records.countOne')
                    : t('records.count', { total })}
              </span>
            </div>
          }
          actions={
            <>
              <Button size='sm' variant='outline' onClick={() => setBatchOpen(true)}>
                <Layers className='size-4' />
                {t('records.addToDdns')}
              </Button>
              <Button
                size='sm'
                onClick={() => {
                  setEditing(null)
                  setDialogOpen(true)
                }}
              >
                <Plus className='size-4' />
                {t('records.addRecord')}
              </Button>
            </>
          }
        />

        {recordsQuery.isError ? (
          <ErrorState onRetry={() => void recordsQuery.refetch()} />
        ) : (
          <RecordsTable
            data={records}
            loading={recordsQuery.isPending}
            actions={actions}
            onBulkDelete={(selected) => {
              // The API refuses a longer list, and a confirmation it cannot carry
              // out is worse than saying so now: the selection is still on screen
              // to narrow, which it is not once the call has come back a 400.
              if (!fitsOneBatch(selected.length)) {
                toast.error(
                  t('records.batchTooLarge', {
                    max: MAX_BATCH_ITEMS,
                    selected: selected.length,
                  })
                )
                return
              }
              setRemoving(selected)
            }}
          />
        )}
      </Main>

      <RecordDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        domain={domain}
        accountId={accountId}
        record={editing}
        saving={saving}
        onSubmit={async (input) => {
          // The failure is already toasted by the global mutation handler, which
          // has the server's own wording; swallowing it here only keeps the
          // rejected promise from surfacing as a second, unhandled error. The
          // dialog stays open either way - only onSuccess closes it.
          await save(input).catch(() => {})
        }}
      />

      <BatchDialog
        open={batchOpen}
        onOpenChange={setBatchOpen}
        saving={creatingMany}
        onSubmit={(items) => {
          createMany(items)
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
            ? t('records.deleteOne', { host: removing[0]?.sub_domain ?? '' })
            : t('records.deleteMany', { count: removing?.length ?? 0 })
        }
        desc={t('records.deleteDescription')}
        confirmText={t('common.delete')}
        // Toasted by the global mutation handler; caught only so the rejection
        // is not left unhandled.
        handleConfirm={() => void removeSelected(removing ?? []).catch(() => {})}
      />
    </>
  )
}
