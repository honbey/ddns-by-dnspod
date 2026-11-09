import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { getDisplayNameInitials } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'

type NavUserProps = {
  user: {
    name: string
    email: string
  }
}

/**
 * Who you are signed in as, and a way into the settings.
 *
 * Everything else lives in the header's profile menu, so the two places cannot
 * drift apart: signing out in particular is offered once.
 */
export function NavUser({ user }: NavUserProps) {
  const { t } = useTranslation()
  const initials = getDisplayNameInitials(user.name)

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton size='lg' asChild tooltip={t('nav.settings')}>
          <Link to='/settings'>
            <Avatar className='h-8 w-8 rounded-lg'>
              <AvatarFallback className='rounded-lg'>{initials}</AvatarFallback>
            </Avatar>
            <div className='grid flex-1 text-start text-sm leading-tight'>
              <span className='truncate font-semibold'>{user.name}</span>
              <span className='truncate text-xs'>{user.email}</span>
            </div>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
