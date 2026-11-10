import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Check, DoorOpen, LogOut, Palette, Settings, UserCog } from 'lucide-react'
import { TranslateIcon } from '@/components/icons/nav-icons'
import { useAuthStore } from '@/stores/auth-store'
import { displayName, type LogoutScope } from '@/lib/auth'
import { getDisplayNameInitials } from '@/lib/utils'
import { LOCALE_LABELS, LOCALES, setLocalePreference } from '@/i18n'
import { useLocalePreference } from '@/hooks/use-locale-preference'
import useDialogState from '@/hooks/use-dialog-state'
import { ConfigDrawer } from '@/components/config-drawer'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { SignOutDialog } from '@/components/sign-out-dialog'

/**
 * The only place sessions are managed: the sidebar just names the user.
 *
 * Both sign-outs are confirmed, because ending the provider's session reaches
 * every other app sharing it and that is not something to do from a misclick.
 */
export function ProfileDropdown() {
  const { t } = useTranslation()
  const [scope, setScope] = useDialogState<LogoutScope>()
  const [configOpen, setConfigOpen] = useState(false)
  const user = useAuthStore((state) => state.auth.user)
  const providerLogout = useAuthStore((state) => state.auth.providerLogout)
  const name = displayName(user)
  const email = user?.profile.email ?? ''
  const initials = getDisplayNameInitials(name)
  const preference = useLocalePreference()

  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <Button
            variant='ghost'
            className='relative h-8 w-8 rounded-full'
            aria-label={t('nav.openAccountMenu', { name })}
          >
            <Avatar className='h-8 w-8'>
              <AvatarFallback>{initials}</AvatarFallback>
            </Avatar>
          </Button>
        </DropdownMenuTrigger>
        {/* No forceMount: a closed menu that stays mounted is still in the
            accessibility tree and still takes clicks, and the closed state only
            animates the opacity away. */}
        <DropdownMenuContent className='w-56' align='end'>
          <DropdownMenuLabel className='font-normal'>
            <div className='flex flex-col gap-1.5'>
              <p className='text-sm leading-none font-medium'>{name}</p>
              <p className='text-muted-foreground text-xs leading-none'>{email}</p>
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            <DropdownMenuItem asChild>
              <Link to='/settings'>
                <UserCog />
                {t('auth.profile')}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to='/settings/appearance'>
                <Palette />
                {t('auth.appearance')}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuSub>
              {/* SubTrigger has no [&_svg] rules of its own, unlike Item, so
                  the mark is sized and toned here to match the two links. */}
              <DropdownMenuSubTrigger>
                <TranslateIcon className='size-4 text-muted-foreground' />
                {t('common.switchLanguage')}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                <DropdownMenuItem
                  onClick={() => setLocalePreference('system')}
                  className={preference === 'system' ? 'font-semibold' : ''}
                >
                  {t('common.followSystem')}
                  {preference === 'system' ? <Check className='ms-auto size-4' /> : null}
                </DropdownMenuItem>
                {LOCALES.map((item) => (
                  <DropdownMenuItem
                    key={item}
                    onClick={() => setLocalePreference(item)}
                    className={preference === item ? 'font-semibold' : ''}
                  >
                    {LOCALE_LABELS[item]}
                    {preference === item ? <Check className='ms-auto size-4' /> : null}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
            <DropdownMenuItem onClick={() => setConfigOpen(true)}>
              <Settings />
              {t('theme.title')}
            </DropdownMenuItem>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant='destructive' onClick={() => setScope('app')}>
            <LogOut />
            {t('auth.signOut')}
          </DropdownMenuItem>
          {providerLogout && (
            <DropdownMenuItem variant='destructive' onClick={() => setScope('provider')}>
              <DoorOpen />
              {t('auth.signOutEverywhere')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <ConfigDrawer open={configOpen} onOpenChange={setConfigOpen} />
      <SignOutDialog scope={scope} onClose={() => setScope(null)} />
    </>
  )
}
