import { useEffect, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { ChevronDownIcon } from '@radix-ui/react-icons'
import { zodResolver } from '@hookform/resolvers/zod'
import { fonts } from '@/config/fonts'
import { LOCALE_LABELS, LOCALES, setLocalePreference, type LocalePreference } from '@/i18n'
import { useLocalePreference } from '@/hooks/use-locale-preference'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useFont } from '@/context/font-provider'
import { useTheme } from '@/context/theme-provider'
import { buttonVariants } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

const appearanceFormSchema = z.object({
  theme: z.enum(['system', 'light', 'dark']),
  font: z.enum(fonts),
})

type AppearanceFormValues = z.infer<typeof appearanceFormSchema>

export function AppearanceForm() {
  const { t } = useTranslation()
  const preference = useLocalePreference()
  const languageLabelId = useId()
  // The theme cards are radio buttons rather than form fields, so the group
  // and each card name their label instead of being pointed at with `for`.
  const themeLabelId = useId()

  const { font, setFont } = useFont()
  const { theme, setTheme } = useTheme()

  const form = useForm<AppearanceFormValues>({
    resolver: zodResolver(appearanceFormSchema),
    defaultValues: { theme, font },
  })

  // Nothing here is submitted: a choice applies the moment it is made, which
  // means the form has to follow the preference from wherever else it changed
  // - the switch in the top bar, the config drawer, or another tab.
  useEffect(() => {
    form.reset({ theme, font })
  }, [theme, font, form])

  function chooseFont(next: AppearanceFormValues['font']) {
    setFont(next)
    toast.success(t('settings.preferencesUpdated'))
  }

  function chooseTheme(next: AppearanceFormValues['theme']) {
    setTheme(next)
    toast.success(t('settings.preferencesUpdated'))
  }

  return (
    <Form {...form}>
      {/* Both choices apply as they are made, so there is nothing to submit. */}
      <form onSubmit={(event) => event.preventDefault()} className='space-y-8'>
        <FormItem>
          <FormLabel id={languageLabelId} htmlFor={undefined}>
            {t('common.language')}
          </FormLabel>
          <div className='relative w-max'>
            <select
              aria-labelledby={languageLabelId}
              className={cn(
                buttonVariants({ variant: 'outline' }),
                'w-50 appearance-none font-normal',
                'dark:bg-background dark:hover:bg-background'
              )}
              value={preference}
              onChange={(event) => setLocalePreference(event.target.value as LocalePreference)}
            >
              <option value='system'>{t('common.followSystem')}</option>
              {LOCALES.map((item) => (
                <option key={item} value={item}>
                  {LOCALE_LABELS[item]}
                </option>
              ))}
            </select>
            <ChevronDownIcon className='absolute inset-e-3 top-2.5 h-4 w-4 opacity-50' />
          </div>
        </FormItem>
        <FormField
          control={form.control}
          name='font'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('settings.fontLabel')}</FormLabel>
              <div className='relative w-max'>
                <FormControl>
                  <select
                    className={cn(
                      buttonVariants({ variant: 'outline' }),
                      'w-50 appearance-none font-normal capitalize',
                      'dark:bg-background dark:hover:bg-background'
                    )}
                    {...field}
                    onChange={(event) => {
                      const next = event.target.value as AppearanceFormValues['font']
                      field.onChange(next)
                      chooseFont(next)
                    }}
                  >
                    {fonts.map((font) => (
                      <option key={font} value={font}>
                        {font}
                      </option>
                    ))}
                  </select>
                </FormControl>
                <ChevronDownIcon className='absolute inset-e-3 top-2.5 h-4 w-4 opacity-50' />
              </div>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='theme'
          render={({ field }) => (
            <FormItem>
              <FormLabel id={themeLabelId} htmlFor={undefined}>
                {t('theme.themeSection')}
              </FormLabel>
              <FormMessage />
              <RadioGroup
                name={field.name}
                aria-labelledby={themeLabelId}
                value={field.value}
                onValueChange={(value) => {
                  const next = value as AppearanceFormValues['theme']
                  field.onChange(next)
                  chooseTheme(next)
                }}
                className='grid max-w-lg grid-cols-3 gap-4 pt-2'
              >
                <FormItem>
                  <FormLabel
                    id={`${themeLabelId}-system`}
                    htmlFor={undefined}
                    className='flex-col items-center gap-0 [&:has([data-state=checked])>div]:border-primary'
                  >
                    <FormControl>
                      <RadioGroupItem
                        value='system'
                        aria-labelledby={`${themeLabelId}-system`}
                        className='sr-only'
                      />
                    </FormControl>
                    <div className='w-full items-center rounded-md border-2 border-muted p-1 hover:border-accent'>
                      <div
                        className='space-y-2 rounded-sm p-2'
                        style={{
                          background: 'linear-gradient(135deg, #ecedef 50%, #0f172a 50%)',
                        }}
                      >
                        <div className='space-y-2 rounded-md bg-white/90 p-2 shadow-xs'>
                          <div className='h-2 w-16 rounded-lg bg-slate-200' />
                          <div className='h-2 w-20 rounded-lg bg-slate-200' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-slate-800/90 p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-slate-400' />
                          <div className='h-2 w-16 rounded-lg bg-slate-400' />
                        </div>
                      </div>
                    </div>
                    <span className='block w-full p-2 text-center font-normal whitespace-nowrap'>
                      {t('common.followSystem')}
                    </span>
                  </FormLabel>
                </FormItem>
                <FormItem>
                  <FormLabel
                    id={`${themeLabelId}-light`}
                    htmlFor={undefined}
                    className='flex-col items-center gap-0 [&:has([data-state=checked])>div]:border-primary'
                  >
                    <FormControl>
                      <RadioGroupItem
                        value='light'
                        aria-labelledby={`${themeLabelId}-light`}
                        className='sr-only'
                      />
                    </FormControl>
                    <div className='w-full items-center rounded-md border-2 border-muted p-1 hover:border-accent'>
                      <div className='space-y-2 rounded-sm bg-[#ecedef] p-2'>
                        <div className='space-y-2 rounded-md bg-white p-2 shadow-xs'>
                          <div className='h-2 w-16 rounded-lg bg-[#ecedef]' />
                          <div className='h-2 w-20 rounded-lg bg-[#ecedef]' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-white p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-[#ecedef]' />
                          <div className='h-2 w-20 rounded-lg bg-[#ecedef]' />
                        </div>
                      </div>
                    </div>
                    <span className='block w-full p-2 text-center font-normal whitespace-nowrap'>
                      {t('theme.light')}
                    </span>
                  </FormLabel>
                </FormItem>
                <FormItem>
                  <FormLabel
                    id={`${themeLabelId}-dark`}
                    htmlFor={undefined}
                    className='flex-col items-center gap-0 [&:has([data-state=checked])>div]:border-primary'
                  >
                    <FormControl>
                      <RadioGroupItem
                        value='dark'
                        aria-labelledby={`${themeLabelId}-dark`}
                        className='sr-only'
                      />
                    </FormControl>
                    <div className='w-full items-center rounded-md border-2 border-muted bg-popover p-1 hover:bg-accent hover:text-accent-foreground'>
                      <div className='space-y-2 rounded-sm bg-slate-950 p-2'>
                        <div className='space-y-2 rounded-md bg-slate-800 p-2 shadow-xs'>
                          <div className='h-2 w-16 rounded-lg bg-slate-400' />
                          <div className='h-2 w-20 rounded-lg bg-slate-400' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-slate-800 p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-slate-400' />
                          <div className='h-2 w-20 rounded-lg bg-slate-400' />
                        </div>
                      </div>
                    </div>
                    <span className='block w-full p-2 text-center font-normal whitespace-nowrap'>
                      {t('theme.dark')}
                    </span>
                  </FormLabel>
                </FormItem>
              </RadioGroup>
            </FormItem>
          )}
        />
      </form>
    </Form>
  )
}
