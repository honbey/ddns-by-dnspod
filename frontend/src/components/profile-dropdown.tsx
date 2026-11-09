import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { BadgeCheck, Check, DoorOpen, Languages, LogOut, Palette } from 'lucide-react'
import { useAuthStore } from '@/stores/auth-store'
import { displayName, type LogoutScope } from '@/lib/auth'
import { getDisplayNameInitials } from '@/lib/utils'
import { currentLocale, LOCALE_LABELS, LOCALES, setLocale } from '@/i18n'
import useDialogState from '@/hooks/use-dialog-state'
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
  const user = useAuthStore((state) => state.auth.user)
  const providerLogout = useAuthStore((state) => state.auth.providerLogout)
  const name = displayName(user)
  const email = user?.profile.email ?? ''
  const initials = getDisplayNameInitials(name)
  const locale = currentLocale()

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
                <BadgeCheck />
                {t('auth.profile')}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to='/settings/appearance'>
                <Palette />
                {t('auth.appearance')}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger>
                <Languages />
                {t('common.language')}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                {LOCALES.map((item) => (
                  <DropdownMenuItem
                    key={item}
                    onClick={() => setLocale(item)}
                    className={locale === item ? 'font-semibold' : ''}
                  >
                    {LOCALE_LABELS[item]}
                    {locale === item ? <Check className='ms-auto size-4' /> : null}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
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

      <SignOutDialog scope={scope} onClose={() => setScope(null)} />
    </>
  )
}
