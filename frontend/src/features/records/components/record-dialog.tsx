import { useTranslation } from 'react-i18next'
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { listRecordLines, type RecordInput } from '@/features/records/api'
import { currentIP } from '@/features/system/api'
import { RECORD_TYPES, TTL_OPTIONS } from '@/lib/cron'
import { MAX_BATCH_ITEMS } from '@/lib/limits'
import { type DnsRecord } from '@/lib/types'
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

/**
 * The types this dialog writes. The batch dialog keeps to the address types,
 * because it fills the value in from this host's detected address.
 */
const WRITABLE_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'HTTPS', 'SVCB'] as const

/** True for the types this dialog can write; the rest are shown as they are. */
function isWritable(type: string): boolean {
  return (WRITABLE_TYPES as readonly string[]).includes(type)
}

/** The types DNSPod refuses without a priority. */
const PRIORITY_TYPES = new Set<string>(['MX', 'HTTPS', 'SVCB'])

/** What the priority starts at, so an MX record is writable without a guess. */
const DEFAULT_PRIORITY = 10

const formSchema = z.object({
  // The API will not file a record with no host, and neither will DNSPod: the
  // apex is written "@", not left blank, so an empty field is a missing answer
  // rather than an implied one.
  sub_domain: z.string().trim().min(1, 'A host is required; @ is the apex.'),
  // Not an enum: a record of a type this dialog does not offer is opened for
  // editing with its own type kept, and telling that apart from an invalid one
  // is the Select's job, not the schema's.
  record_type: z.string().trim().min(1, 'A type is required.'),
  value: z.string().trim().min(1, 'A value is required.'),
  line: z.string(),
  ttl: z.number().int().positive(),
  mx: z.number().int().min(0),
  weight: z.number().int().min(0),
  status: z.enum(['ENABLE', 'DISABLE']),
  remark: z.string(),
})

type FormValues = z.infer<typeof formSchema>

const EMPTY: FormValues = {
  sub_domain: '',
  record_type: 'A',
  value: '',
  line: '',
  ttl: 600,
  mx: DEFAULT_PRIORITY,
  weight: 0,
  status: 'ENABLE',
  remark: '',
}

function valuesFor(record: DnsRecord | null): FormValues {
  if (!record) return EMPTY
  return {
    sub_domain: record.sub_domain,
    // Kept as it is, rather than folded back onto A: a record of a type this
    // dialog cannot write would otherwise be saved over the original as an
    // address record. The Type control is read-only for those, so the value
    // cannot drift while it sits in the form.
    record_type: record.record_type,
    value: record.value,
    line: record.line,
    ttl: record.ttl || 600,
    // A priority of zero is not one, and only the types DNSPod requires one for
    // carry it. Defaulting the rest would put a setting on them they do not
    // have, which is exactly what the dialog must not do to a type it cannot
    // write in the first place.
    mx: record.mx || 0,
    weight: record.weight || 0,
    status: record.status === 'DISABLE' ? 'DISABLE' : 'ENABLE',
    remark: record.remark,
  }
}

type RecordDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  domain: string
  /** Which account holds the domain: its lines are read per account too. */
  accountId?: number
  /** The record being edited, or null when a new one is being added. */
  record: DnsRecord | null
  saving: boolean
  onSubmit: (values: RecordInput) => Promise<void>
}

/**
 * One record.
 *
 * The value is an address in practice, so the host's own address is offered as
 * the starting point rather than typed from memory.
 */
export function RecordDialog({
  open,
  onOpenChange,
  domain,
  accountId,
  record,
  saving,
  onSubmit,
}: RecordDialogProps) {
  const { t } = useTranslation()
  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: EMPTY,
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(record))
  }, [open, record, form])

  const type = form.watch('record_type')
  const typeWritable = isWritable(type)
  // Only an address record has anything to say about this host's address
  const addressType = type === 'A' || type === 'AAAA'
  const lines = useQuery({
    queryKey: ['lines', domain, accountId],
    queryFn: () => listRecordLines(domain, accountId),
    enabled: open,
  })
  const detected = useQuery({
    queryKey: ['ip', type === 'A' ? 'v4' : 'v6'],
    queryFn: () => currentIP(type === 'A' ? 'v4' : 'v6'),
    enabled: open && addressType,
  })

  function submit(values: FormValues) {
    // Checked here rather than left to the API: a zero priority is not one, and
    // sending it resets the priority a record already has.
    if (PRIORITY_TYPES.has(values.record_type) && !values.mx) {
      form.setError('mx', {
        message: t('records.priorityRequired'),
      })
      return
    }
    // An edit has to name its line. Blank is what a create means by "the
    // default", and letting an edit say the same thing would move a record that
    // lives on another line without anything on screen saying so.
    if (record && !values.line.trim()) {
      form.setError('line', { message: t('records.lineRequired') })
      return
    }
    void onSubmit({
      sub_domain: values.sub_domain,
      record_type: values.record_type,
      value: values.value,
      line: values.line,
      ttl: values.ttl,
      // A record that takes no priority gets none: a fresh MX record starts at
      // DEFAULT_PRIORITY, and anything else sends zero, which the API reads as
      // "leave what is there alone".
      mx: PRIORITY_TYPES.has(values.record_type) ? values.mx : 0,
      weight: values.weight,
      status: values.status,
      remark: values.remark,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full gap-0 p-0 sm:max-w-lg'>
        <SheetHeader className='border-b pe-10'>
          <SheetTitle>{record ? t('records.editTitle') : t('records.addTitle')}</SheetTitle>
          <SheetDescription>{t('records.dialogHint')}</SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} className='flex min-h-0 flex-1 flex-col'>
            <div className='min-h-0 flex-1 space-y-6 overflow-y-auto p-4'>
              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='sub_domain'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('records.hostLabel')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('records.hostPlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>{t('records.hostHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='record_type'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('records.typeLabel')}</FormLabel>
                      {typeWritable ? (
                        <Select
                          name={field.name}
                          value={field.value}
                          onValueChange={field.onChange}
                        >
                          <FormControl>
                            <SelectTrigger className='w-full'>
                              <SelectValue />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {WRITABLE_TYPES.map((option) => (
                              <SelectItem key={option} value={option}>
                                {option}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        // Shown as it is, not as a choice: offering only the
                        // types this dialog can write would let the record be
                        // re-typed the moment it is saved.
                        //
                        // readOnly, not disabled: the type cannot change, but
                        // it must stay in the tab order and copyable, which is
                        // what the a11y pass gave every other control here.
                        <FormControl>
                          <Input
                            name={field.name}
                            value={field.value}
                            readOnly
                            aria-label={t('records.typeAria')}
                            className='bg-muted text-muted-foreground font-mono'
                          />
                        </FormControl>
                      )}
                      <FormDescription>
                        {typeWritable
                          ? t('records.typeHint')
                          : `${type} records are saved with their own type; the rest of the record is still yours to change.`}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='value'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('records.valueLabel')}</FormLabel>
                    <FormControl>
                      <Input {...field} className='font-mono' />
                    </FormControl>
                    <FormDescription>
                      {addressType && detected.data?.available ? (
                        <button
                          type='button'
                          className='underline underline-offset-4'
                          onClick={() => field.onChange(detected.data?.ip)}
                        >
                          Use this host&apos;s address ({detected.data.ip})
                        </button>
                      ) : addressType ? (
                        t('records.valueHintAddress')
                      ) : (
                        t('records.valueHintText')
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Only these types carry one, and DNSPod refuses them without it. */}
              {PRIORITY_TYPES.has(type) && (
                <FormField
                  control={form.control}
                  name='mx'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('records.priorityLabel')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={1}
                          {...field}
                          onChange={(event) => field.onChange(Number(event.target.value) || 0)}
                        />
                      </FormControl>
                      <FormDescription>{t('records.priorityHint', { type })}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <div className='grid items-start gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='line'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('records.lineLabel')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('tasks.linePlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>
                        {lines.data?.length
                          ? `one of ${(lines.data ?? []).map((line) => line.name).join(', ')}`
                          : record
                            ? t('records.lineHintEdit')
                            : t('records.lineHintCreate')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='ttl'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('records.ttlColumn')}</FormLabel>
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
                      <FormDescription>{t('records.ttlHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='weight'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('records.weightLabel')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          {...field}
                          onChange={(event) => field.onChange(Number(event.target.value) || 0)}
                        />
                      </FormControl>
                      <FormDescription>{t('records.weightHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='remark'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('records.remarkLabel')}</FormLabel>
                    <FormControl>
                      <Input placeholder={t('notifications.optionalPlaceholder')} {...field} />
                    </FormControl>
                    <FormDescription>{t('records.remarkHint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                    <div className='space-y-0.5'>
                      <FormLabel>{t('records.enabledLabel')}</FormLabel>
                      <FormDescription>{t('records.enabledHint')}</FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value === 'ENABLE'}
                        onCheckedChange={(checked) =>
                          field.onChange(checked ? 'ENABLE' : 'DISABLE')
                        }
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
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
                {record ? t('common.saveChanges') : t('records.addTitle')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}

type BatchDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  saving: boolean
  onSubmit: (items: RecordInput[]) => void
}

/**
 * Several hosts at once, each pointing at whatever this host has right now.
 *
 * One per line, because that is how a list of hostnames arrives - pasted from a
 * note rather than typed one dialog at a time.
 */
export function BatchDialog({ open, onOpenChange, saving, onSubmit }: BatchDialogProps) {
  const { t } = useTranslation()
  const [hosts, setHosts] = useState('')
  const [recordType, setRecordType] = useState<'A' | 'AAAA'>('A')

  const detected = useQuery({
    queryKey: ['ip', recordType === 'A' ? 'v4' : 'v6'],
    queryFn: () => currentIP(recordType === 'A' ? 'v4' : 'v6'),
    enabled: open,
  })

  // Both halves of the form, not just the hosts: a type picked last time would
  // otherwise stay chosen when the dialog is opened again.
  useEffect(() => {
    if (open) {
      setHosts('')
      setRecordType('A')
    }
  }, [open])

  // Capped to what the API accepts in one call (maxBatchItems). The overflow is
  // counted rather than dropped silently: a paste of 150 hosts used to report
  // "100 records to create" and quietly lose the rest.
  const pasted = hosts
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  const names = pasted.slice(0, MAX_BATCH_ITEMS)
  const dropped = pasted.length - names.length

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full gap-0 p-0 sm:max-w-lg'>
        <SheetHeader className='border-b pe-10'>
          <SheetTitle>{t('records.batchTitle')}</SheetTitle>
          <SheetDescription>
            One host per line, each pointed at {detected.data?.ip || 'this host'}.
          </SheetDescription>
        </SheetHeader>

        <div className='flex-1 space-y-6 overflow-y-auto p-4'>
          <div className='space-y-2'>
            <label htmlFor='batch-hosts' className='text-sm font-medium'>
              {t('records.hostsLabel')}
            </label>
            <Textarea
              id='batch-hosts'
              rows={8}
              placeholder={'home\nvpn\nnas'}
              value={hosts}
              onChange={(event) => setHosts(event.target.value)}
            />
            <p className='text-muted-foreground text-xs'>
              {dropped > 0
                ? `Only the first ${MAX_BATCH_ITEMS} of ${pasted.length} will be created.`
                : names.length
                  ? `${names.length} record${names.length === 1 ? '' : 's'} to create`
                  : 'Nothing yet.'}
            </p>
          </div>

          <div className='space-y-2'>
            <label htmlFor='batch-type' className='text-sm font-medium'>
              {t('records.typeLabel')}
            </label>
            <Select
              value={recordType}
              onValueChange={(value) => setRecordType(value as 'A' | 'AAAA')}
            >
              <SelectTrigger id='batch-type' className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RECORD_TYPES.map((option) => (
                  <SelectItem key={option} value={option}>
                    {option}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>{t('records.typeHint')}</p>
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
          <Button
            size='sm'
            disabled={saving || names.length === 0 || !detected.data?.ip}
            onClick={() =>
              onSubmit(
                names.map((name) => ({
                  sub_domain: name,
                  record_type: recordType,
                  value: detected.data?.ip ?? '',
                  line: '',
                  ttl: 600,
                  mx: 0,
                  status: 'ENABLE',
                  remark: '',
                }))
              )
            }
          >
            {saving && <Loader2 className='animate-spin' />}
            Create {names.length || ''}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
