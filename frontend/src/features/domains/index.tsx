import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, ListTree, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { listAccounts } from '@/features/accounts/api'
import { listDomains } from '@/features/domains/api'
import { listRecordGroups } from '@/features/records/api'
import { importTasksFromGroup } from '@/features/tasks/api'
import { handleServerError } from '@/lib/handle-server-error'
import { cronPresets, RECORD_TYPES, describeCron } from '@/lib/cron'
import { type Domain } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { DomainsTable } from './components/domains-table'

/**
 * The domains the configured accounts can change.
 *
 * Imported from DNSPod rather than entered, because a name this deployment does
 * not control cannot be pointed anywhere.
 */
export function Domains() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: listAccounts })
  // Empty means the default account, which is what the API answers with when a
  // request names none. The picker is how the other accounts' domains become
  // reachable at all - they used to be missing from this page entirely.
  const [picked, setPicked] = useState('')
  const accountId = picked ? Number(picked) : 0
  const domains = useQuery({
    queryKey: ['domains', accountId],
    queryFn: () => listDomains(accountId || undefined),
  })
  const [importFor, setImportFor] = useState<Domain | null>(null)

  const accountList = accounts.data ?? []
  const defaultAccount = accountList.find((account) => account.is_default) ?? accountList[0]

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4'>
        <PageHeading
          title={t('nav.domains')}
          subtitle={t('domains.subtitle')}
          actions={
            <>
              {accountList.length > 1 ? (
                <Select value={picked} onValueChange={setPicked}>
                  <SelectTrigger className='w-56' aria-label={t('domains.accountAria')}>
                    <SelectValue placeholder={t('domains.accountPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {accountList.map((account) => (
                      <SelectItem key={account.id} value={String(account.id)}>
                        {account.name}
                        {account.is_default ? t('domains.defaultSuffix') : ''}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : null}
              <Button
                size='sm'
                variant='outline'
                disabled={domains.isFetching}
                onClick={() => domains.refetch()}
              >
                <RefreshCw className={domains.isFetching ? 'animate-spin' : undefined} />
                {t('domains.refresh')}
              </Button>
            </>
          }
        />

        {domains.isError ? (
          <ErrorState onRetry={() => void domains.refetch()} />
        ) : (
          <DomainsTable
            data={domains.data ?? []}
            loading={domains.isPending}
            actions={{ onImport: setImportFor, accountId }}
          />
        )}
      </Main>

      {/* Keyed by the domain: the dialog stays mounted while it is closed, so
          without this the group and account picked for one domain are still
          there when it is opened for the next. */}
      <ImportDialog
        key={importFor?.name ?? 'none'}
        domain={importFor}
        defaultAccountId={accountId || defaultAccount?.id}
        onClose={() => setImportFor(null)}
        onDone={async () => {
          await queryClient.invalidateQueries({ queryKey: ['tasks'] })
          setImportFor(null)
        }}
      />
    </>
  )
}

type ImportDialogProps = {
  domain: Domain | null
  /** The account the page is showing; the picker below starts on it. */
  defaultAccountId?: number
  onClose: () => void
  onDone: () => Promise<void>
}

/**
 * Turns a DNSPod record group into tasks.
 *
 * A group is how DNSPod itself bundles records, so importing one is often how a
 * whole set of hosts gets tracked in a single step. Records that already have a
 * task are skipped, which makes repeating it harmless.
 */
function ImportDialog({ domain, defaultAccountId, onClose, onDone }: ImportDialogProps) {
  const { t } = useTranslation()
  const [groupId, setGroupId] = useState('')
  const [accountId, setAccountId] = useState(defaultAccountId ? String(defaultAccountId) : '')
  const [recordType, setRecordType] = useState<string>('A')
  const [cronExpr, setCronExpr] = useState('*/10 * * * *')

  const accounts = useQuery({
    queryKey: ['accounts'],
    queryFn: () => listAccounts(),
    enabled: Boolean(domain),
  })
  // The groups come from the account the import would actually use. Asked
  // against the default account, another account's domain answers with whatever
  // that account happens to hold under the same name - usually nothing.
  const groups = useQuery({
    queryKey: ['groups', domain?.name, accountId],
    queryFn: () => listRecordGroups(domain!.name, Number(accountId)),
    enabled: Boolean(domain) && Boolean(accountId),
  })

  const { mutate, isPending } = useMutation({
    mutationFn: () =>
      importTasksFromGroup(domain!.name, {
        group_id: Number(groupId),
        account_id: Number(accountId),
        record_type: recordType,
        cron_expr: cronExpr,
        enabled: true,
      }),
    onSuccess: async (result) => {
      await onDone()
      toast.success(
        result.skipped
          ? t('domains.importCreatedWithSkipped', {
              created: result.created,
              skipped: result.skipped,
            })
          : t('domains.importCreated', { created: result.created })
      )
    },
    onError: (error) => handleServerError(error, t('domains.importFailed')),
  })

  const ready = Boolean(groupId && accountId)

  return (
    <Sheet
      open={domain !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent className='w-full gap-0 p-0 sm:max-w-lg'>
        <SheetHeader className='border-b pe-10'>
          <SheetTitle>{t('domains.importFromGroup')}</SheetTitle>
          <SheetDescription>
            Every address record of that group in {domain?.name} becomes a task.
          </SheetDescription>
        </SheetHeader>

        <div className='flex-1 space-y-6 overflow-y-auto p-4'>
          <Field
            label={t('domains.accountAria')}
            hint={t('domains.importAccountHint')}
            htmlFor='import-account'
          >
            <Select
              value={accountId}
              onValueChange={(value) => {
                setAccountId(value)
                // The stored group belongs to the account it was read from.
                setGroupId('')
              }}
            >
              <SelectTrigger id='import-account' className='w-full'>
                <SelectValue placeholder={t('domains.whichAccount')} />
              </SelectTrigger>
              <SelectContent>
                {(accounts.data ?? []).map((account) => (
                  <SelectItem key={account.id} value={String(account.id)}>
                    {account.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field
            label={t('domains.recordGroup')}
            hint={t('domains.recordGroupHint')}
            htmlFor='import-group'
          >
            <Select value={groupId} onValueChange={setGroupId}>
              <SelectTrigger id='import-group' className='w-full'>
                <SelectValue placeholder={t('domains.whichGroup')} />
              </SelectTrigger>
              <SelectContent>
                {(groups.data ?? []).map((group) => (
                  <SelectItem key={group.id} value={String(group.id)}>
                    {group.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <div className='grid items-start gap-4 sm:grid-cols-[9rem_1fr]'>
            <Field
              label={t('domains.type')}
              hint={t('domains.addressOnlyHint')}
              htmlFor='import-type'
            >
              <Select value={recordType} onValueChange={setRecordType}>
                <SelectTrigger id='import-type' className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RECORD_TYPES.map((type) => (
                    <SelectItem key={type} value={type}>
                      {type}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field
              label={t('domains.schedule')}
              hint={describeCron(cronExpr)}
              htmlFor='import-schedule'
            >
              <Select value={cronExpr} onValueChange={setCronExpr}>
                <SelectTrigger id='import-schedule' className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {cronPresets.map((preset) => (
                    <SelectItem key={preset.value} value={preset.value}>
                      {preset.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </div>
        </div>

        <SheetFooter className='border-t'>
          <Button size='sm' variant='outline' disabled={isPending} onClick={onClose}>
            {t('common.close')}
          </Button>
          <Button size='sm' disabled={!ready || isPending} onClick={() => mutate()}>
            {isPending ? <Loader2 className='animate-spin' /> : <ListTree className='size-4' />}
            {t('domains.import')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

/** A labelled control. The label is a real one, tied to the control by id: a
 * trigger announces only its current value, so without this a screen reader
 * never hears what the field is for. */
function Field({
  label,
  hint,
  htmlFor,
  children,
}: {
  label: string
  hint?: string
  htmlFor: string
  children: React.ReactNode
}) {
  return (
    <div className='space-y-2'>
      <div className='space-y-0.5'>
        <label htmlFor={htmlFor} className='text-sm font-medium'>
          {label}
        </label>
        {hint ? <p className='text-muted-foreground text-xs'>{hint}</p> : null}
      </div>
      {children}
    </div>
  )
}
