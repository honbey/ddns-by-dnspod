import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, PowerOff } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { listAccounts } from '@/features/accounts/api'
import {
  createCertificate,
  deleteCertificate,
  issueCertificate,
  listCertificates,
  type CertificateInput,
  updateCertificate,
} from '@/features/certificates/api'
import { handleServerError } from '@/lib/handle-server-error'
import { acmeEnabled } from '@/lib/config'
import { type Certificate } from '@/lib/types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ErrorState } from '@/components/error-state'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { CertificateDialog } from './components/certificate-dialog'
import { type CertificateRowActions } from './components/certificates-columns'
import { CertificatesTable } from './components/certificates-table'

/**
 * The certificates this deployment issues and keeps renewed.
 *
 * Everything here is about the files: they are written with the names certbot
 * uses, into a directory the TLS application on the other side mounts, so
 * nothing has to be copied by hand between a renewal and the thing that serves
 * it.
 */
export function Certificates() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const certificates = useQuery({
    queryKey: ['certificates'],
    queryFn: listCertificates,
    // An issuance runs in the background, so this list is the only place its
    // progress shows: while one is running, it is re-read until it settles.
    refetchInterval: (query) =>
      (query.state.data ?? []).some((cert) => cert.last_status === 'RUNNING') ? 2000 : false,
  })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: listAccounts })

  // What this deployment is allowed to do, from the runtime configuration the
  // bundle loaded before rendering: the buttons follow that instead of finding
  // out from a 503 the first time somebody presses one.
  const canIssue = acmeEnabled()

  const [editing, setEditing] = useState<Certificate | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [removing, setRemoving] = useState<Certificate | null>(null)

  const accountNames = useMemo(
    () => new Map((accounts.data ?? []).map((account) => [account.id, account.name])),
    [accounts.data]
  )

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['certificates'] })

  const { mutateAsync: save, isPending: saving } = useMutation({
    mutationFn: async ({ id, input }: { id?: number; input: CertificateInput }) =>
      id ? updateCertificate(id, input) : createCertificate(input),
    onSuccess: async (cert) => {
      await invalidate()
      setDialogOpen(false)
      toast.success(
        `Saved ${cert.name}. ${
          cert.not_after ? t('certificates.renewHint') : t('certificates.issueHint')
        }`
      )
    },
  })

  const { mutate: issue } = useMutation({
    mutationFn: (id: number) => issueCertificate(id),
    onSuccess: async (cert) => {
      await invalidate()
      toast.success(`Issuing ${cert.name}...`)
    },
    onError: (error) => handleServerError(error, t('certificates.issueFailed')),
  })

  const { mutateAsync: remove, isPending: removingPending } = useMutation({
    mutationFn: (cert: Certificate) => deleteCertificate(cert.id),
    onSuccess: () => toast.success(t('certificates.removed')),
    onSettled: async () => {
      // Runs on failure too, so a failure after part of the work has gone
      // through does not leave the table showing a row the server dropped.
      await invalidate()
      setRemoving(null)
    },
  })

  const actions = useMemo<CertificateRowActions>(
    () => ({
      onIssue: (cert) => issue(cert.id),
      onEdit: (cert) => {
        setEditing(cert)
        setDialogOpen(true)
      },
      onDelete: (cert) => setRemoving(cert),
    }),
    [issue]
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
          title={t('nav.certificates')}
          subtitle={t('certificates.subtitle')}
          actions={
            <Button
              size='sm'
              onClick={() => {
                setEditing(null)
                setDialogOpen(true)
              }}
            >
              <Plus className='size-4' />
              {t('certificates.add')}
            </Button>
          }
        />

        {!canIssue && (
          <Alert>
            <PowerOff />
            <AlertTitle>{t('certificates.issuerOffTitle')}</AlertTitle>
            <AlertDescription>{t('certificates.issuerOffDescription')}</AlertDescription>
          </Alert>
        )}

        {certificates.isError ? (
          <ErrorState onRetry={() => void certificates.refetch()} />
        ) : (
          <CertificatesTable
            data={certificates.data ?? []}
            loading={certificates.isPending}
            actions={actions}
            accountNames={accountNames}
            canIssue={canIssue}
          />
        )}
      </Main>

      <CertificateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        certificate={editing}
        saving={saving}
        canIssue={canIssue}
        onSubmit={async (input) => {
          try {
            await save({ id: editing?.id, input })
          } catch {
            // Swallowed: the global mutation handler has already reported the
            // failure with the server's own wording, and the dialog is left
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
        title={t('certificates.removeOne', { name: removing?.name ?? '' })}
        desc={t('certificates.removeDescription')}
        confirmText={t('common.remove')}
        // Toasted by the global mutation handler; caught only so the rejection
        // is not left unhandled.
        handleConfirm={() => {
          if (removing) void remove(removing).catch(() => {})
        }}
      />
    </>
  )
}
