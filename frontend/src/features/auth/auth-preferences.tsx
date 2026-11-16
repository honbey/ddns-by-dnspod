import { useTranslation } from 'react-i18next'
import { Check, Moon, Sun } from 'lucide-react'
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
import { LOCALE_LABELS, LOCALES, setLocalePreference } from '@/i18n'
import { type Theme } from '@/lib/theme'

const THEMES: Theme[] = ['system', 'light', 'dark']

/**
 * Language and theme, on the page that has no account menu yet.
 *
 * The choices land in the same storage the app reads after sign-in, so what a
 * visitor picks here is what the dashboard opens with.
 */
export function AuthPreferences() {
  const { t } = useTranslation()
  const { resolvedTheme, theme, setTheme } = useTheme()
  const preference = useLocalePreference()

  const themeLabels: Record<Theme, string> = {
    system: t('common.followSystem'),
    light: t('theme.light'),
    dark: t('theme.dark'),
  }

  return (
    <div className='flex items-center gap-1'>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant='ghost'
            size='icon'
            className='rounded-full'
            aria-label={t('common.switchLanguage')}
          >
            <TranslateIcon className='size-[1.2rem]' />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
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

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant='ghost'
            size='icon'
            className='rounded-full'
            aria-label={t('theme.selectTheme')}
          >
            {resolvedTheme === 'dark' ? (
              <Moon className='size-[1.2rem]' />
            ) : (
              <Sun className='size-[1.2rem]' />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
          {THEMES.map((item) => (
            <DropdownMenuItem
              key={item}
              onClick={() => setTheme(item)}
              className={theme === item ? 'font-semibold' : ''}
            >
              {themeLabels[item]}
              {theme === item ? <Check className='ms-auto size-4' /> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
