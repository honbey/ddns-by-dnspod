import { type ElementType } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronsUpDown, Monitor, Moon, Sun } from 'lucide-react'
import { TranslateIcon } from '@/components/icons/nav-icons'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/context/theme-provider'
import { useLocalePreference } from '@/hooks/use-locale-preference'
import { currentLocale, LOCALE_LABELS, LOCALES, setLocalePreference } from '@/i18n'
import { cn } from '@/lib/utils'
import { type Theme } from '@/lib/theme'

const THEMES: Theme[] = ['system', 'light', 'dark']
const THEME_ICONS: Record<Theme, ElementType> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
}

/**
 * Language and theme, on the page that has no account menu yet.
 *
 * The choices land in the same storage the app reads after sign-in, so what a
 * visitor picks here is what the dashboard opens with.
 */
export function AuthPreferences() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const preference = useLocalePreference()

  const themeLabels: Record<Theme, string> = {
    system: t('common.followSystem'),
    light: t('theme.light'),
    dark: t('theme.dark'),
  }

  return (
    <div className='flex items-center justify-between gap-3 border-t bg-muted/30 px-4 py-3'>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant='ghost'
            size='sm'
            className='h-8 gap-2 px-2 font-normal text-muted-foreground hover:text-foreground'
            aria-label={t('common.switchLanguage')}
          >
            <TranslateIcon className='size-4' />
            <span>{LOCALE_LABELS[currentLocale()]}</span>
            <ChevronsUpDown className='size-3.5 opacity-60' />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='start' side='top' className='min-w-44'>
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
        </DropdownMenuContent>
      </DropdownMenu>

      <div
        role='group'
        aria-label={t('theme.selectTheme')}
        className='flex items-center gap-0.5 rounded-md bg-muted p-0.5'
      >
        {THEMES.map((item) => {
          const Icon = THEME_ICONS[item]
          const selected = theme === item
          return (
            <Button
              key={item}
              type='button'
              variant='ghost'
              size='icon'
              aria-label={themeLabels[item]}
              aria-pressed={selected}
              title={themeLabels[item]}
              onClick={() => setTheme(item)}
              className={cn(
                'size-7 rounded-sm',
                selected
                  ? 'bg-background text-foreground shadow-sm ring-1 ring-border/60 hover:bg-background dark:bg-accent dark:ring-border dark:hover:bg-accent'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              <Icon className='size-4' />
            </Button>
          )
        })}
      </div>
    </div>
  )
}
