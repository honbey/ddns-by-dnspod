import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { displayName, profileOf, type LogoutScope } from '@/lib/auth'
import { providerName } from '@/lib/config'
import { getDisplayNameInitials } from '@/lib/utils'
import useDialogState from '@/hooks/use-dialog-state'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SignOutDialog } from '@/components/sign-out-dialog'
import { ContentSection } from '../components/content-section'

/**
 * Who the identity provider says you are.
 *
 * Nothing here is editable: the identity belongs to the provider, and the app
 * merely reads it off the token - which is why the page needs no endpoint of
 * its own and works no matter which provider is configured.
 */
export function SettingsProfile() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const providerLogout = useAuthStore((state) => state.auth.providerLogout)
  const [scope, setScope] = useDialogState<LogoutScope>()

  const profile = profileOf(user)
  const name = displayName(user)
  const email = profile.email ?? ''
  const initials = getDisplayNameInitials(name)
  const provider = providerName()

  return (
    <ContentSection title={t('nav.profile')} desc={t('settings.profileDescription')}>
      <div className='space-y-6'>
        <div className='flex items-center gap-4'>
          <Avatar className='h-14 w-14'>
            <AvatarFallback className='text-lg'>{initials}</AvatarFallback>
          </Avatar>
          <div className='space-y-1'>
            <p className='text-lg font-medium'>{name}</p>
            <p className='text-sm text-muted-foreground'>{email || '-'}</p>
          </div>
        </div>

        <Separator />

        {/* The two short facts share the first row; the subject is long enough
            to need the whole of the second one. */}
        <dl className='grid gap-4 text-sm sm:grid-cols-2'>
          <div className='space-y-1'>
            <dt className='text-muted-foreground'>{t('settings.providerLabel')}</dt>
            <dd>{provider}</dd>
          </div>
          <div className='space-y-1'>
            <dt className='text-muted-foreground'>{t('settings.groupsLabel')}</dt>
            <dd className='flex flex-wrap gap-1'>
              {profile.groups?.length ? (
                profile.groups.map((group) => (
                  <Badge key={group} variant='secondary'>
                    {group}
                  </Badge>
                ))
              ) : (
                <span className='text-muted-foreground'>{t('ui.none')}</span>
              )}
            </dd>
          </div>
          <div className='space-y-1 sm:col-span-2'>
            <dt className='text-muted-foreground'>{t('settings.subjectLabel')}</dt>
            <dd className='break-all font-mono'>{profile.sub ?? '-'}</dd>
          </div>
        </dl>

        <Separator />

        {/* One button fits beside the explanation. A second one needs its own
            row, or the two would squeeze the paragraph. */}
        <div className={providerLogout ? 'space-y-3' : 'flex items-center justify-between gap-4'}>
          <p className='text-sm text-muted-foreground'>{t('auth.signOutNote', { provider })}</p>
          <div
            className={
              providerLogout ? 'flex flex-wrap items-center gap-2' : 'flex shrink-0 items-center'
            }
          >
            <Button
              variant='outline'
              className='text-destructive hover:text-destructive'
              onClick={() => setScope('app')}
            >
              {t('auth.signOut')}
            </Button>
            {providerLogout && (
              <Button
                variant='outline'
                className='text-destructive hover:text-destructive'
                onClick={() => setScope('provider')}
              >
                {t('auth.signOutEverywhere')}
              </Button>
            )}
          </div>
        </div>

        <SignOutDialog scope={scope} onClose={() => setScope(null)} />
      </div>
    </ContentSection>
  )
}
