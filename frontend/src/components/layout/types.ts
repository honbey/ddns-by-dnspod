import { type LinkProps } from '@tanstack/react-router'
import { type MessageKey } from '@/i18n'

/**
 * A navigation item carries a message key rather than its text: the sidebar is
 * static data built once at module load, so holding the translated string here
 * would freeze it in whichever language happened to be active then and it
 * would not follow a switch.
 */
type BaseNavItem = {
  titleKey: MessageKey
  badge?: string
  icon?: React.ElementType
}

type NavLink = BaseNavItem & {
  url: LinkProps['to'] | (string & {})
  items?: never
}

type NavCollapsible = BaseNavItem & {
  items: (BaseNavItem & { url: LinkProps['to'] | (string & {}) })[]
  url?: never
}

type NavItem = NavCollapsible | NavLink

type NavGroup = {
  titleKey: MessageKey
  items: NavItem[]
}

type SidebarData = {
  navGroups: NavGroup[]
}

export type { SidebarData, NavGroup, NavItem, NavCollapsible, NavLink }
