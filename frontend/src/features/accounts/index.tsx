import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import {
  createAccount,
  deleteAccount,
  listAccounts,
  type AccountInput,
  updateAccount,
  verifyAccount,
} from '@/features/accounts/api'
import { handleServerError } from '@/lib/handle-server-error'
import { clearMissingAccountNotice } from '@/lib/query-client'
import { type Account } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { AccountDialog } from './components/account-dialog'
import { type AccountRowActions } from './components/accounts-columns'
import { AccountsTable } from './components/accounts-table'

/**
 * Credentials, kept apart from everything else they unlock.
 *
 * Verify exists because the alternative is writing a dummy record to see whether
 * it fails; DNSPod is asked directly instead.
 */
export function Accounts() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: () => listAccounts() })

  const [editing, setEditing] = useState<Account | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [removing, setRemoving] = useState<Account[] | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['accounts'] })

  const { mutateAsync: save, isPending: saving } = useMutation({
    mutationFn: async ({ id, input }: { id?: number; input: AccountInput }) =>
      id ? updateAccount(id, input) : createAccount(input),
    onSuccess: async () => {
      // The one-shot warning was about the empty list this just filled.
      clearMissingAccountNotice()
      await invalidate()
      // The domain list depends on which credentials exist, so it goes stale here.
      await queryClient.invalidateQueries({ queryKey: ['domains'] })
      setDialogOpen(false)
      toast.success(t('accounts.saved'))
    },
  })

  const { mutate: verify } = useMutation({
    mutationFn: (id: number) => verifyAccount(id),
    onSuccess: () => toast.success(t('accounts.verified')),
    onError: (error) => handleServerError(error, t('accounts.refused')),
  })

  const { mutate: remove, isPending: removingPending } = useMutation({
    mutationFn: async (selected: Account[]) => {
      for (const account of selected) await deleteAccount(account.id)
    },
    onSuccess: () => toast.success(t('accounts.removed')),
    onSettled: async () => {
      // Runs on failure too; see removeSelected in records for the reasoning.
      await invalidate()
      setRemoving(null)
    },
    onError: (error) => handleServerError(error, t('accounts.removeInUse')),
  })

  const { mutate: verifyMany } = useMutation({
    mutationFn: async (selected: Account[]) => {
      // Verdicts are per account and each one is worth hearing, so failures
      // here surface one toast each rather than one aggregated error.
      const results = await Promise.allSettled(selected.map((account) => verifyAccount(account.id)))
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') {
          toast.success(`${selected[index].name}: credentials accepted.`)
        } else {
          handleServerError(result.reason, `${selected[index].name}: refused.`)
        }
      })
    },
  })

  const actions = useMemo<AccountRowActions>(
    () => ({
      onEdit: (account) => {
        setEditing(account)
        setDialogOpen(true)
      },
      onVerify: (account) => verify(account.id),
      onDelete: (account) => setRemoving([account]),
    }),
    [verify]
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
          title={t('nav.accounts')}
          subtitle={t('accounts.subtitle')}
          actions={
            <Button
              size='sm'
              onClick={() => {
                setEditing(null)
                setDialogOpen(true)
              }}
            >
              <Plus className='size-4' />
              {t('accounts.add')}
            </Button>
          }
        />

        {accounts.isError ? (
          <ErrorState onRetry={() => void accounts.refetch()} />
        ) : (
          <AccountsTable
            data={accounts.data ?? []}
            loading={accounts.isPending}
            actions={actions}
            onBulkVerify={(selected) => verifyMany(selected)}
            onBulkDelete={setRemoving}
          />
        )}
      </Main>

      <AccountDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        account={editing}
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
            ? t('accounts.removeOne', { name: removing[0]?.name ?? '' })
            : t('accounts.removeMany', { count: removing?.length ?? 0 })
        }
        desc={t('accounts.removeDescription')}
        confirmText={t('common.remove')}
        handleConfirm={() => {
          if (removing) remove(removing)
        }}
      />
    </>
  )
}
