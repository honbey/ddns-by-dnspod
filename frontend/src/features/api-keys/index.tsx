import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { listAccounts } from '@/features/accounts/api'
import {
  createAPIKey,
  deleteAPIKey,
  listAPIKeys,
  type APIKeyInput,
  updateAPIKey,
} from '@/features/api-keys/api'
import { handleServerError } from '@/lib/handle-server-error'
import { type APIKey } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { APIKeyCreatedDialog } from './components/api-key-created'
import { APIKeyDialog } from './components/api-key-dialog'
import { type APIKeyRowActions } from './components/api-keys-columns'
import { APIKeysTable } from './components/api-keys-table'

/**
 * Credentials for scripts.
 *
 * Issued rather than edited into existence: the plaintext comes back once, at
 * creation, because only a prefix is stored.
 */
export function APIKeys() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const keys = useQuery({ queryKey: ['keys'], queryFn: listAPIKeys })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: () => listAccounts() })

  const [editing, setEditing] = useState<APIKey | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [issued, setIssued] = useState<{ name: string; domain: string; secret: string } | null>(
    null
  )
  const [removing, setRemoving] = useState<APIKey[] | null>(null)

  const accountNames = useMemo(
    () => new Map((accounts.data ?? []).map((account) => [account.id, account.name])),
    [accounts.data]
  )

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['keys'] })

  const { mutateAsync: save, isPending: saving } = useMutation({
    mutationFn: async ({ id, input }: { id?: number; input: APIKeyInput }) =>
      id ? updateAPIKey(id, input) : createAPIKey(input),
    onSuccess: async (result) => {
      await invalidate()
      setDialogOpen(false)
      if ('key' in result) {
        // The one moment the plaintext exists, so it is surfaced before the list
        // is even looked at again.
        setIssued({
          name: result.api_key.name,
          domain: result.api_key.domain,
          secret: result.key,
        })
        return
      }
      toast.success(t('apiKeys.saved'))
    },
  })

  const { mutateAsync: toggle } = useMutation({
    mutationFn: (apiKey: APIKey) => updateAPIKey(apiKey.id, { enabled: !apiKey.enabled }),
    onSuccess: async () => {
      await invalidate()
      toast.success(t('apiKeys.switched'))
    },
    onError: (error) => handleServerError(error, t('apiKeys.switchFailed')),
  })

  const { mutateAsync: remove, isPending: removingPending } = useMutation({
    mutationFn: async (selected: APIKey[]) => {
      for (const apiKey of selected) await deleteAPIKey(apiKey.id)
    },
    onSuccess: () => toast.success(t('apiKeys.removed')),
    onSettled: async () => {
      // Runs on failure too; see removeSelected in records for the reasoning.
      await invalidate()
      setRemoving(null)
    },
  })

  const actions = useMemo<APIKeyRowActions>(
    () => ({
      onEdit: (apiKey) => {
        setEditing(apiKey)
        setDialogOpen(true)
      },
      onToggle: (apiKey) => toggle(apiKey),
      onDelete: (apiKey) => setRemoving([apiKey]),
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
          title={t('nav.apiKeys')}
          subtitle={t('apiKeys.subtitle')}
          actions={
            <Button
              size='sm'
              onClick={() => {
                setEditing(null)
                setDialogOpen(true)
              }}
            >
              <Plus className='size-4' />
              {t('apiKeys.createButton')}
            </Button>
          }
        />

        {keys.isError ? (
          <ErrorState onRetry={() => void keys.refetch()} />
        ) : (
          <APIKeysTable
            data={keys.data ?? []}
            loading={keys.isPending}
            actions={actions}
            accountNames={accountNames}
            onBulkSetEnabled={(selected, enabled) =>
              selected
                .filter((apiKey) => apiKey.enabled !== enabled)
                // Toasted by the mutation's own onError; caught only so the
                // rejection is not left unhandled.
                .forEach((apiKey) => void toggle(apiKey).catch(() => {}))
            }
            onBulkDelete={setRemoving}
          />
        )}
      </Main>

      <APIKeyDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        apiKey={editing}
        saving={saving}
        onSubmit={async (input) => {
          // Toasted by the global mutation handler with the server's wording;
          // caught only so the rejection is not left unhandled.
          await save({ id: editing?.id, input }).catch(() => {})
        }}
      />

      <APIKeyCreatedDialog
        open={issued !== null}
        onOpenChange={(open) => {
          if (!open) setIssued(null)
        }}
        domain={issued?.domain ?? ''}
        secret={issued?.secret ?? ''}
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
            ? `Revoke ${removing[0]?.name}?`
            : `Revoke ${removing?.length ?? 0} keys?`
        }
        desc={t('apiKeys.revokeDescription')}
        confirmText={t('apiKeys.revoke')}
        // Toasted by the global mutation handler; caught only so the rejection
        // is not left unhandled.
        handleConfirm={() => void remove(removing ?? []).catch(() => {})}
      />
    </>
  )
}
