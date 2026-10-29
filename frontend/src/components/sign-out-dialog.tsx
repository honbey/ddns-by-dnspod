import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { signOut, type LogoutScope } from '@/lib/auth'
import { getConfig, providerName } from '@/lib/config'
import { ConfirmDialog } from '@/components/confirm-dialog'

interface SignOutDialogProps {
  /** Which session to end; null keeps the dialog closed. */
  scope: LogoutScope | null
  onClose: () => void
}

export function SignOutDialog({ scope, onClose }: SignOutDialogProps) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const { appName } = getConfig()
  const provider = providerName()
  const everywhere = scope === 'provider'

  const handleConfirm = () => {
    if (!scope) return
    setPending(true)
    // The sign-out ends in a navigation, so the dialog simply stays disabled
    // until the browser leaves the page.
    void signOut(scope).catch(() => setPending(false))
  }

  return (
    <ConfirmDialog
      open={scope !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
      title={everywhere ? t('auth.signOutEverywhereQuestion') : t('auth.signOut')}
      desc={
        everywhere
          ? t('auth.signOutEverywhereDescription', { provider })
          : t('auth.signOutDescription', { app: appName, provider })
      }
      confirmText={everywhere ? t('auth.signOutEverywhere') : t('auth.signOut')}
      destructive
      isLoading={pending}
      handleConfirm={handleConfirm}
      className='sm:max-w-sm'
    />
  )
}
