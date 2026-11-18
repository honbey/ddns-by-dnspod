import { useTranslation } from 'react-i18next'
import { type TFunction } from 'i18next'
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { listAccounts } from '@/features/accounts/api'
import { type CertificateInput } from '@/features/certificates/api'
import { defaultRenewCron, defaultRenewHour } from '@/lib/config'
import { certificateCronPresets, describeCron, isValidCron } from '@/lib/cron'
import { type Certificate } from '@/lib/types'
import { FieldHelp } from '@/components/field-help'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { KEY_TYPES, formatNames, parseNames } from './certificate-types'

/** The name rule the API applies, spelled out here so the form says it first. */
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/

function certificateSchema(t: TFunction) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t('common.nameRequired'))
      .regex(NAME_RE, t('certificates.nameRequired')),
    account_id: z.number().int().positive(t('common.selectAccount')),
    domains: z.string().trim().min(1, t('certificates.domainsRequired')),
    key_type: z.string().trim().min(1, t('certificates.keyTypeRequired')),
    output_dir: z.string().trim(),
    renew_cron: z
      .string()
      .trim()
      .refine(
        // @every is an interval, not a schedule: the server ticks once a minute
        // and matches the expression against it, so an interval would never fire
        // (or fire on every tick). Five fields and the calendar descriptors do.
        (value) => value === '' || (!value.trim().startsWith('@every') && isValidCron(value)),
        t('certificates.renewCronInvalid')
      ),
    // One message for every way the number can be wrong: zod's own wording is
    // English, and the input is held to a whole number so it never reaches that.
    renew_before_days: z
      .number()
      .refine(
        (value) => Number.isInteger(value) && value >= 0 && value <= 60,
        t('certificates.renewBeforeRange')
      ),
    notify_on_issue: z.boolean(),
  })
}

/** What the expression means, when it means something other than itself. */
function cronWords(expr: string, t: TFunction): string {
  const trimmed = expr.trim()
  if (!trimmed) return t('certificates.renewManual')
  const words = describeCron(trimmed, t)
  return words === trimmed ? '' : words
}

type FormValues = z.infer<ReturnType<typeof certificateSchema>>

/** The blank form, with the deployment's own starting schedule. */
function emptyValues(): FormValues {
  return {
    name: '',
    account_id: 0,
    domains: '',
    key_type: 'ec256',
    output_dir: '',
    renew_cron: defaultRenewCron(),
    renew_before_days: 0,
    notify_on_issue: true,
  }
}

function valuesFor(cert: Certificate | null): FormValues {
  if (!cert) return emptyValues()
  return {
    name: cert.name,
    account_id: cert.account_id,
    domains: formatNames(cert.domains),
    key_type: cert.key_type,
    output_dir: cert.output_dir,
    renew_cron: cert.renew_cron,
    renew_before_days: cert.renew_before_days,
    notify_on_issue: cert.notify_on_issue,
  }
}

type CertificateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The certificate being edited, or null when a new one is being added. */
  certificate: Certificate | null
  saving: boolean
  /** False while this deployment has issuance switched off. */
  canIssue: boolean
  onSubmit: (values: CertificateInput) => Promise<void>
}

/**
 * One certificate: which names it covers, and who answers for them.
 *
 * Nothing here asks for the private key or the challenge: both exist only
 * inside the issuance, and the point of the feature is that nobody has to carry
 * them between two systems by hand.
 */
export function CertificateDialog({
  open,
  onOpenChange,
  certificate,
  saving,
  canIssue,
  onSubmit,
}: CertificateDialogProps) {
  const { t } = useTranslation()
  // The shortcuts name the hour the deployment checks at, not a second copy of
  // it: clicking one must not move a schedule to another hour.
  const presets = certificateCronPresets(defaultRenewHour())
  const form = useForm<FormValues>({
    resolver: zodResolver(certificateSchema(t)),
    defaultValues: emptyValues(),
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(certificate))
  }, [open, certificate, form])

  const accounts = useQuery({
    queryKey: ['accounts'],
    queryFn: () => listAccounts(),
    enabled: open,
  })

  function submit(values: FormValues) {
    void onSubmit({
      name: values.name,
      account_id: values.account_id,
      domains: parseNames(values.domains),
      key_type: values.key_type,
      output_dir: values.output_dir,
      renew_cron: values.renew_cron,
      renew_before_days: values.renew_before_days,
      notify_on_issue: values.notify_on_issue,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full gap-0 p-0 sm:max-w-xl'>
        <SheetHeader className='pe-10'>
          <SheetTitle>{t('certificates.add')}</SheetTitle>
          <SheetDescription>{t('certificates.dialogHint')}</SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} className='flex min-h-0 flex-1 flex-col'>
            <div className='min-h-0 flex-1 space-y-6 overflow-y-auto p-4'>
              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('common.name')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('certificates.namePlaceholder')} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='account_id'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('ui.account')}</FormLabel>
                      <Select
                        name={field.name}
                        value={field.value ? String(field.value) : ''}
                        onValueChange={(value) => field.onChange(Number(value))}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue placeholder={t('certificates.accountPlaceholder')} />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {(accounts.data ?? []).map((account) => (
                            <SelectItem key={account.id} value={String(account.id)}>
                              {account.name}
                              {account.is_default ? t('common.defaultSuffix') : ''}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='domains'
                render={({ field }) => (
                  <FormItem>
                    <div className='flex items-center gap-1.5'>
                      <FormLabel>{t('certificates.domainsLabel')}</FormLabel>
                      <FieldHelp field={t('certificates.domainsLabel')}>
                        {t('certificates.domainsTooltip')}
                      </FieldHelp>
                    </div>
                    <FormControl>
                      <Textarea
                        rows={4}
                        placeholder={'example.com\nwww.example.com'}
                        className='font-mono text-sm'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='key_type'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('certificates.keyTypeLabel')}</FormLabel>
                      <Select name={field.name} value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {KEY_TYPES.map((option) => (
                            <SelectItem key={option.id} value={option.id}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='output_dir'
                  render={({ field }) => (
                    <FormItem>
                      <div className='flex items-center gap-1.5'>
                        <FormLabel>{t('certificates.directoryLabel')}</FormLabel>
                        <FieldHelp field={t('certificates.directoryLabel')}>
                          {t('certificates.outputDirHint')}
                        </FieldHelp>
                      </div>
                      <FormControl>
                        <Input placeholder={t('certificates.namePlaceholder')} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='renew_cron'
                render={({ field }) => (
                  <FormItem>
                    <div className='flex items-center gap-1.5'>
                      <FormLabel>{t('certificates.renewCronLabel')}</FormLabel>
                      <FieldHelp field={t('certificates.renewCronLabel')} announce={false}>
                        {canIssue
                          ? t('certificates.renewCronHint')
                          : t('certificates.renewHintOff')}
                      </FieldHelp>
                    </div>
                    <FormControl>
                      <Input
                        {...field}
                        className='font-mono'
                        placeholder={defaultRenewCron()}
                        disabled={!canIssue}
                      />
                    </FormControl>
                    {/* The words only when they say more than the expression
                        already does; an unknown schedule is left to the input,
                        without an empty line under it. */}
                    {cronWords(field.value, t) ? (
                      <FormDescription>{cronWords(field.value, t)}</FormDescription>
                    ) : null}
                    <div className='flex flex-wrap gap-2 pt-1'>
                      {presets.map((preset) => (
                        <Button
                          key={preset.value}
                          type='button'
                          size='sm'
                          variant='outline'
                          disabled={!canIssue}
                          onClick={() =>
                            form.setValue('renew_cron', preset.value, { shouldValidate: true })
                          }
                        >
                          {t(preset.key)}
                        </Button>
                      ))}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* The window and the switch are the same height once the hint
                  moves behind its button: the two controls line up. */}
              <div className='grid items-end gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='renew_before_days'
                  render={({ field }) => (
                    <FormItem>
                      <div className='flex items-center gap-1.5'>
                        <FormLabel>{t('certificates.renewBeforeLabel')}</FormLabel>
                        <FieldHelp field={t('certificates.renewBeforeLabel')}>
                          {t('certificates.renewBeforeHint')}
                        </FieldHelp>
                      </div>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          max={60}
                          value={field.value}
                          onChange={(event) => {
                            const next = Number(event.target.value)
                            field.onChange(Number.isFinite(next) ? next : 0)
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='notify_on_issue'
                  render={({ field }) => (
                    <FormItem className='flex h-9 items-center justify-between gap-3 space-y-0 rounded-md border px-3'>
                      <div className='flex items-center gap-1.5'>
                        <FormLabel>{t('certificates.notifyLabel')}</FormLabel>
                        <FieldHelp field={t('certificates.notifyLabel')}>
                          {t('certificates.notifyHint')}
                        </FieldHelp>
                      </div>
                      <FormControl>
                        <Switch checked={field.value} onCheckedChange={field.onChange} />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>
            </div>

            <SheetFooter className='border-t'>
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={saving}
                onClick={() => onOpenChange(false)}
              >
                {t('common.close')}
              </Button>
              <Button type='submit' size='sm' disabled={saving}>
                {saving && <Loader2 className='animate-spin' />}
                {t('common.confirm')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
