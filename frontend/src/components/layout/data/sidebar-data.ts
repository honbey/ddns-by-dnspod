import {
  BadgeCheck,
  BellRing,
  Globe,
  History,
  KeyRound,
  LayoutDashboard,
  ListChecks,
  Palette,
  Settings,
  ShieldCheck,
  UserCog,
} from 'lucide-react'
import { type SidebarData } from '../types'

/**
 * What is reachable from the sidebar.
 *
 * Everything a person would act on lives in the header instead: the bottom of
 * the sidebar names who is signed in, and the account menu in the top right is
 * the only place with anything to click.
 *
 * The titles are message keys, not text - see `NavGroup` for why the
 * translation has to happen at render.
 */
export const sidebarData: SidebarData = {
  navGroups: [
    {
      titleKey: 'nav.dynamicDns',
      items: [
        {
          titleKey: 'nav.dashboard',
          url: '/',
          icon: LayoutDashboard,
        },
        {
          titleKey: 'nav.tasks',
          url: '/tasks',
          icon: ListChecks,
        },
        {
          titleKey: 'nav.domains',
          url: '/domains',
          icon: Globe,
        },
        {
          titleKey: 'nav.history',
          url: '/history',
          icon: History,
        },
      ],
    },
    {
      titleKey: 'nav.configuration',
      items: [
        {
          titleKey: 'nav.accounts',
          url: '/accounts',
          icon: ShieldCheck,
        },
        {
          titleKey: 'nav.certificates',
          url: '/certificates',
          icon: BadgeCheck,
        },
        {
          titleKey: 'nav.apiKeys',
          url: '/api-keys',
          icon: KeyRound,
        },
      ],
    },
    {
      titleKey: 'nav.other',
      items: [
        {
          titleKey: 'nav.notifications',
          url: '/notifications',
          icon: BellRing,
        },
        {
          titleKey: 'nav.settings',
          icon: Settings,
          items: [
            {
              titleKey: 'nav.profile',
              url: '/settings',
              icon: UserCog,
            },
            {
              titleKey: 'nav.appearance',
              url: '/settings/appearance',
              icon: Palette,
            },
          ],
        },
      ],
    },
  ],
}
