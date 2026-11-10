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
import { type Certificate } from '@/lib/types'
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
      .min(1, t('certificates.nameRequiredEmpty'))
      .regex(NAME_RE, t('certificates.nameRequired')),
    account_id: z.number().int().positive(t('certificates.accountRequired')),
    domains: z.string().trim().min(1, t('certificates.domainsRequired')),
    key_type: z.string().trim().min(1, t('certificates.keyTypeRequired')),
    output_dir: z.string().trim(),
    auto_renew: z.boolean(),
    notify_on_issue: z.boolean(),
  })
}

type FormValues = z.infer<ReturnType<typeof certificateSchema>>

const EMPTY: FormValues = {
  name: '',
  account_id: 0,
  domains: '',
  key_type: 'ec256',
  output_dir: '',
  auto_renew: true,
  notify_on_issue: true,
}

function valuesFor(cert: Certificate | null): FormValues {
  if (!cert) return EMPTY
  return {
    name: cert.name,
    account_id: cert.account_id,
    domains: formatNames(cert.domains),
    key_type: cert.key_type,
    output_dir: cert.output_dir,
    auto_renew: cert.auto_renew,
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
  const form = useForm<FormValues>({
    resolver: zodResolver(certificateSchema(t)),
    defaultValues: EMPTY,
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(certificate))
  }, [open, certificate, form])

  const rawNames = form.watch('domains')
  const names = parseNames(rawNames)

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
      auto_renew: values.auto_renew,
      notify_on_issue: values.notify_on_issue,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full gap-0 p-0 sm:max-w-xl'>
        <SheetHeader className='border-b pe-10'>
          <SheetTitle>
            {certificate ? t('certificates.editTitle') : t('certificates.addTitle')}
          </SheetTitle>
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
                      <FormLabel>{t('certificates.nameLabel')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('certificates.namePlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>{t('certificates.directoryHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='account_id'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('certificates.accountColumn')}</FormLabel>
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
                              {account.is_default ? ' (default)' : ''}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>{t('certificates.accountHint')}</FormDescription>
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
                    <FormLabel>{t('certificates.domainsLabel')}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={4}
                        placeholder={'example.com\nwww.example.com'}
                        className='font-mono text-sm'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('certificates.domainsHint')}{' '}
                      {names.length > 0
                        ? t('certificates.domainsCount', { count: names.length })
                        : t('certificates.domainsSubject')}{' '}
                      {t('certificates.domainsWildcard')} <code>*.example.com</code>.
                    </FormDescription>
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
                      <FormLabel>{t('ui.key')}</FormLabel>
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
                      <FormDescription>
                        {KEY_TYPES.find((option) => option.id === field.value)?.hint ?? ''}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='output_dir'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('certificates.directoryLabel')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('certificates.namePlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>{t('certificates.outputDirHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='auto_renew'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('certificates.renewLabel')}</FormLabel>
                        <FormDescription>
                          {canIssue
                            ? t('certificates.renewHintOn')
                            : t('certificates.renewHintOff')}
                        </FormDescription>
                      </div>
                      <FormControl>
                        {/* Kept as it stands rather than switched off: it is
                            what will happen once issuance is turned back on. */}
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          disabled={!canIssue}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='notify_on_issue'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('certificates.notifyLabel')}</FormLabel>
                        <FormDescription>{t('certificates.notifyHint')}</FormDescription>
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
                {t('common.cancel')}
              </Button>
              <Button type='submit' size='sm' disabled={saving}>
                {saving && <Loader2 className='animate-spin' />}
                {certificate ? t('common.saveChanges') : t('certificates.addTitle')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
