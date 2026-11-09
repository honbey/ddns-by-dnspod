import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Moon, Sun } from 'lucide-react'
import { useTheme } from '@/context/theme-provider'
import { Button } from '@/components/ui/button'

/**
 * One button, two themes: clicking it is the switch.
 *
 * Following the machine is a choice as well, but it is a settled one rather
 * than a daily one, so it lives in the config drawer and the command menu
 * instead of behind this button.
 *
 * The resolved theme, not the preference, is what the button reads: with the
 * preference set to "system" the label and the icon have to name what is on
 * screen, and clicking has to move away from it.
 */
export function ThemeSwitch() {
  const { t } = useTranslation()
  const { resolvedTheme, setTheme } = useTheme()

  /* Update theme-color meta tag
   * when theme is updated */
  useEffect(() => {
    const themeColor = resolvedTheme === 'dark' ? '#020817' : '#fff'
    const metaThemeColor = document.querySelector("meta[name='theme-color']")
    if (metaThemeColor) metaThemeColor.setAttribute('content', themeColor)
  }, [resolvedTheme])

  const next = resolvedTheme === 'dark' ? 'light' : 'dark'

  return (
    <Button
      variant='ghost'
      size='icon'
      className='scale-95 rounded-full'
      onClick={() => setTheme(next)}
    >
      <Sun className='size-[1.2rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90' />
      <Moon className='absolute size-[1.2rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0' />
      <span className='sr-only'>{t('common.switchTheme', { theme: next })}</span>
    </Button>
  )
}
