import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { type TFunction } from 'i18next'
import { Loader2 } from 'lucide-react'
import { listAccounts } from '@/features/accounts/api'
import { listDomains } from '@/features/domains/api'
import { listRecordLines } from '@/features/records/api'
import { type TaskInput } from '@/features/tasks/api'
import { cronPresets, describeCron, isValidCron, RECORD_TYPES, TTL_OPTIONS } from '@/lib/cron'
import { type DDNSTask } from '@/lib/types'
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

/** Checked field by field in lib/cron, the way the scheduler reads it. */

/**
 * Built per render so the validation messages are translated too.
 */
function taskSchema(t: TFunction) {
  return z.object({
    account_id: z.number().int().positive(t('tasks.accountRequired')),
    domain: z.string().trim().min(1, t('tasks.domainRequired')),
    sub_domain: z.string().trim(),
    record_type: z.enum(RECORD_TYPES),
    record_line: z.string(),
    ttl: z.number().int().positive(),
    cron_expr: z
      .string()
      .trim()
      .min(1, t('tasks.scheduleRequired'))
      .refine(isValidCron, t('tasks.scheduleInvalid')),
    enabled: z.boolean(),
    notify_on_change: z.boolean(),
  })
}

type FormValues = z.infer<ReturnType<typeof taskSchema>>

const EMPTY: FormValues = {
  account_id: 0,
  domain: '',
  sub_domain: '',
  record_type: 'A',
  record_line: '',
  ttl: 600,
  cron_expr: '*/5 * * * *',
  enabled: true,
  notify_on_change: false,
}

function valuesFor(task: DDNSTask | null): FormValues {
  if (!task) return EMPTY
  return {
    account_id: task.account_id,
    domain: task.domain,
    sub_domain: task.sub_domain,
    record_type: (RECORD_TYPES as readonly string[]).includes(task.record_type)
      ? (task.record_type as FormValues['record_type'])
      : 'A',
    record_line: task.record_line,
    ttl: task.ttl || 600,
    cron_expr: task.cron_expr,
    enabled: task.enabled,
    notify_on_change: task.notify_on_change,
  }
}

type TaskDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The task being edited, or null when a new one is being added. */
  task: DDNSTask | null
  saving: boolean
  onSubmit: (values: TaskInput) => Promise<void>
}

/**
 * One task: which record, and how often it is compared against the address this
 * host has.
 *
 * The address itself is never entered - a task exists to follow it - which is
 * why nothing here asks for one.
 */
export function TaskDialog({ open, onOpenChange, task, saving, onSubmit }: TaskDialogProps) {
  const { t } = useTranslation()
  const form = useForm<FormValues>({
    resolver: zodResolver(taskSchema(t)),
    defaultValues: EMPTY,
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(task))
  }, [open, task, form])

  const domain = form.watch('domain')
  const accountId = form.watch('account_id')

  const accounts = useQuery({
    queryKey: ['accounts'],
    queryFn: () => listAccounts(),
    enabled: open,
  })
  // The domains come from DNSPod itself, so the list cannot offer a name this
  // deployment does not control - and it is read for the account the task would
  // use: a name held by another account could be picked and would then fail on
  // every run, because that is where the credentials point.
  const domains = useQuery({
    queryKey: ['domains', accountId],
    queryFn: () => listDomains(accountId || undefined),
    enabled: open && accountId > 0,
  })
  // Which lines exist differs per domain: some regions have more than the default.
  const lines = useQuery({
    queryKey: ['lines', domain, accountId],
    queryFn: () => listRecordLines(domain, accountId || undefined),
    enabled: open && domain.length > 0,
  })

  const lineNames = lines.data?.map((line) => line.name) ?? []
  const knownDomains = domains.data ?? []

  function submit(values: FormValues) {
    void onSubmit({
      account_id: values.account_id,
      domain: values.domain,
      sub_domain: values.sub_domain,
      record_type: values.record_type,
      record_line: values.record_line,
      ttl: values.ttl,
      cron_expr: values.cron_expr.trim(),
      enabled: values.enabled,
      notify_on_change: values.notify_on_change,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full gap-0 p-0 sm:max-w-xl'>
        <SheetHeader className='border-b pe-10'>
          <SheetTitle>{task ? t('tasks.editTitle') : t('tasks.addTitle')}</SheetTitle>
          <SheetDescription>
            A task compares one record against this host&apos;s public address and updates it when
            they differ.
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} className='flex min-h-0 flex-1 flex-col'>
            <div className='min-h-0 flex-1 space-y-6 overflow-y-auto p-4'>
              <FormField
                control={form.control}
                name='account_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('tasks.accountColumn')}</FormLabel>
                    <Select
                      name={field.name}
                      value={field.value ? String(field.value) : ''}
                      onValueChange={(value) => {
                        field.onChange(Number(value))
                        // Both belong to the account: the domain list is read
                        // from it, and the line exists inside the domain.
                        form.setValue('domain', '')
                        form.setValue('record_line', '')
                      }}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue placeholder={t('tasks.accountPlaceholder')} />
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
                    <FormDescription>{t('tasks.accountHint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid items-start gap-4 sm:grid-cols-[1fr_9rem]'>
                <FormField
                  control={form.control}
                  name='domain'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('tasks.domainLabel')}</FormLabel>
                      {knownDomains.length > 0 ? (
                        <Select
                          name={field.name}
                          value={field.value}
                          onValueChange={field.onChange}
                        >
                          <FormControl>
                            <SelectTrigger className='w-full'>
                              <SelectValue placeholder={t('tasks.domainPlaceholder')} />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {knownDomains.map((item) => (
                              <SelectItem key={item.name} value={item.name}>
                                {item.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <FormControl>
                          <Input placeholder={t('tasks.domainPlaceholder')} {...field} />
                        </FormControl>
                      )}
                      <FormDescription>
                        {knownDomains.length === 0
                          ? t('tasks.domainManualHint')
                          : t('tasks.domainListHint')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='record_type'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('tasks.typeColumn')}</FormLabel>
                      <Select name={field.name} value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {RECORD_TYPES.map((type) => (
                            <SelectItem key={type} value={type}>
                              {type}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>{t('tasks.typeHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='sub_domain'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('tasks.subdomainLabel')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('tasks.subdomainPlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>{t('tasks.subdomainHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='record_line'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('tasks.lineLabel')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('tasks.linePlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>
                        {lineNames.length
                          ? `one of ${lineNames.slice(0, 3).join(', ')}`
                          : t('tasks.lineHint')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='cron_expr'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('tasks.scheduleLabel')}</FormLabel>
                      <FormControl>
                        <Input {...field} className='font-mono' />
                      </FormControl>
                      <FormDescription>{describeCron(field.value)}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className='space-y-2'>
                  <span className='text-muted-foreground text-sm leading-none'>
                    {t('tasks.presets')}
                  </span>
                  <div className='flex flex-wrap gap-2'>
                    {cronPresets.map((preset) => (
                      <Button
                        key={preset.value}
                        type='button'
                        size='sm'
                        variant='outline'
                        onClick={() =>
                          form.setValue('cron_expr', preset.value, {
                            shouldValidate: true,
                          })
                        }
                      >
                        {preset.label}
                      </Button>
                    ))}
                  </div>
                </div>
              </div>

              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='ttl'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('tasks.ttlLabel')}</FormLabel>
                      <Select
                        name={field.name}
                        value={String(field.value)}
                        onValueChange={(value) => field.onChange(Number(value))}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {TTL_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={String(option.value)}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>{t('tasks.ttlHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className='space-y-4'>
                  <FormField
                    control={form.control}
                    name='enabled'
                    render={({ field }) => (
                      <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                        <div className='space-y-0.5'>
                          <FormLabel>{t('tasks.enabledLabel')}</FormLabel>
                          <FormDescription>{t('tasks.enabledHint')}</FormDescription>
                        </div>
                        <FormControl>
                          <Switch checked={field.value} onCheckedChange={field.onChange} />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='notify_on_change'
                    render={({ field }) => (
                      <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                        <div className='space-y-0.5'>
                          <FormLabel>{t('tasks.notifyLabel')}</FormLabel>
                          <FormDescription>{t('tasks.notifyHint')}</FormDescription>
                        </div>
                        <FormControl>
                          <Switch checked={field.value} onCheckedChange={field.onChange} />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                </div>
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
                {task ? t('common.saveChanges') : t('tasks.add')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
