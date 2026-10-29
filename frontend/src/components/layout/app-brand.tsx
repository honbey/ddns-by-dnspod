import { Logo } from '@/assets/logo'
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'

/**
 * What this deployment is, where the title used to be.
 *
 * Nothing to pick here - one server answers this app - so it is a label rather
 * than a menu: the sidebar stays about what the user can do, and identity gets
 * one place only.
 */
export function AppBrand() {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton size='lg' asChild>
          {/* A div rather than the button this renders by default: there is
              nothing to click, and a button that does nothing is a focusable
              tab stop that lies about what it is. */}
          <div>
            {/* The button forces direct svg children to 16px, so the logo sits in
                a wrapper of its own - same trick the backend switcher uses. */}
            <div className='flex size-8 shrink-0 items-center justify-center'>
              <Logo className='size-full' />
            </div>
            <div className='grid flex-1 text-start text-sm leading-tight'>
              <span className='truncate font-bold'>DDNS</span>
              <span className='text-muted-foreground truncate text-xs'>by DNSPod</span>
            </div>
          </div>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
