import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { useAuthStore } from '@/stores/auth-store'
import { displayName, profileOf, signOut } from '@/lib/auth'
import { providerName } from '@/lib/config'
import { getDisplayNameInitials } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { ContentSection } from '../components/content-section'

/**
 * Who the identity provider says you are.
 *
 * Nothing here is editable: the identity belongs to the provider, and the app
 * merely reads it off the token - which is why the page needs no endpoint of
 * its own and works no matter which provider is configured.
 */
export function SettingsProfile() {
  const user = useAuthStore((state) => state.auth.user)
  const [signingOut, setSigningOut] = useState(false)

  const profile = profileOf(user)
  const name = displayName(user)
  const email = profile.email ?? ''
  const initials = getDisplayNameInitials(name)
  const provider = providerName()
  const { t } = useTranslation()

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

        <dl className='grid gap-4 text-sm sm:grid-cols-2'>
          <div className='space-y-1'>
            <dt className='text-muted-foreground'>{t('settings.subjectLabel')}</dt>
            <dd className='break-all font-mono'>{profile.sub ?? '-'}</dd>
          </div>
          <div className='space-y-1'>
            <dt className='text-muted-foreground'>{t('settings.providerLabel')}</dt>
            <dd>{provider}</dd>
          </div>
          <div className='space-y-1 sm:col-span-2'>
            <dt className='text-muted-foreground'>{t('settings.groupsLabel')}</dt>
            <dd className='flex flex-wrap gap-1'>
              {profile.groups?.length ? (
                profile.groups.map((group) => (
                  <Badge key={group} variant='secondary'>
                    {group}
                  </Badge>
                ))
              ) : (
                <span className='text-muted-foreground'>none</span>
              )}
            </dd>
          </div>
        </dl>

        <Separator />

        <div className='flex items-center justify-between gap-4'>
          <p className='text-sm text-muted-foreground'>
            Signing out ends this browser&apos;s session and revokes this application&apos;s access.
            Your {provider} session stays active; use &ldquo;Sign Out Everywhere&rdquo; in the user
            menu to end that too.
          </p>
          <Button
            variant='outline'
            disabled={signingOut}
            onClick={() => {
              setSigningOut(true)
              void signOut().catch(() => setSigningOut(false))
            }}
          >
            {signingOut && <Loader2 className='animate-spin' />}
            Sign Out
          </Button>
        </div>
      </div>
    </ContentSection>
  )
}
