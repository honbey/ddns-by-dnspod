import React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRight, ChevronRight, Laptop, Moon, Sun } from 'lucide-react'
import { useSearch } from '@/context/search-provider'
import { useTheme } from '@/context/theme-provider'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { sidebarData } from './layout/data/sidebar-data'

export function CommandMenu() {
  const navigate = useNavigate()
  const { setTheme } = useTheme()
  const { open, setOpen } = useSearch()
  const { t } = useTranslation()

  const runCommand = React.useCallback(
    (command: () => unknown) => {
      setOpen(false)
      command()
    },
    [setOpen]
  )

  return (
    <CommandDialog modal open={open} onOpenChange={setOpen}>
      {/* The dialog's sr-only title names the dialog, not this field */}
      <CommandInput placeholder={t('nav.searchPlaceholder')} aria-label={t('nav.searchCommands')} />
      {/* The list is the scroller: cmdk scrolls it to keep the highlighted item
          in view, so a ScrollArea inside it would hide that item below a second
          scrollbar - and its forced overflow-x adds a horizontal one. */}
      <CommandList>
        <CommandEmpty>{t('nav.noResults')}</CommandEmpty>
        {sidebarData.navGroups.map((group) => (
          <CommandGroup
            key={group.titleKey ?? group.items[0]?.titleKey ?? 'group'}
            heading={group.titleKey ? t(group.titleKey) : undefined}
          >
            {group.items.map((navItem, i) => {
              if (navItem.url)
                return (
                  <CommandItem
                    key={`${navItem.url}-${i}`}
                    value={t(navItem.titleKey)}
                    onSelect={() => {
                      runCommand(() => navigate({ to: navItem.url }))
                    }}
                  >
                    <div className='flex size-4 items-center justify-center'>
                      <ArrowRight className='size-2 text-muted-foreground/80' />
                    </div>
                    {t(navItem.titleKey)}
                  </CommandItem>
                )

              return navItem.items?.map((subItem, i) => (
                <CommandItem
                  key={`${navItem.titleKey}-${subItem.url}-${i}`}
                  value={`${t(navItem.titleKey)}-${subItem.url}`}
                  onSelect={() => {
                    runCommand(() => navigate({ to: subItem.url }))
                  }}
                >
                  <div className='flex size-4 items-center justify-center'>
                    <ArrowRight className='size-2 text-muted-foreground/80' />
                  </div>
                  {t(navItem.titleKey)} <ChevronRight /> {t(subItem.titleKey)}
                </CommandItem>
              ))
            })}
          </CommandGroup>
        ))}
        <CommandSeparator />
        <CommandGroup heading={t('theme.themeSection')}>
          <CommandItem onSelect={() => runCommand(() => setTheme('light'))}>
            <Sun /> <span>{t('theme.light')}</span>
          </CommandItem>
          <CommandItem onSelect={() => runCommand(() => setTheme('dark'))}>
            <Moon className='scale-90' />
            <span>{t('theme.dark')}</span>
          </CommandItem>
          <CommandItem onSelect={() => runCommand(() => setTheme('system'))}>
            <Laptop />
            <span>{t('common.followSystem')}</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
