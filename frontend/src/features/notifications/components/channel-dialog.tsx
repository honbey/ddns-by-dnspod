import { useTranslation } from 'react-i18next'
import { type TFunction } from 'i18next'
import { useEffect } from 'react'
import { z } from 'zod'
import { useForm, useFieldArray } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2, Plus, Trash2 } from 'lucide-react'
import { type ChannelInput } from '@/features/notifications/api'
import { type NotifyChannel } from '@/lib/types'
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
import { CHANNEL_TYPES, headersOf, ntfyOf, webhookOf } from './channel-types'

/** ntfy priorities, from the lowest to the one that overrides Do Not Disturb. */
const PRIORITY_VALUES = ['5', '4', '3', '2', '1'] as const

const METHODS = ['POST', 'PUT', 'PATCH']

/** Built per render so the validation messages are translated too. */
function channelSchema(t: TFunction) {
  return z
    .object({
      name: z.string().trim().min(1, t('notifications.nameRequired')),
      type: z.enum(['ntfy', 'webhook']),
      enabled: z.boolean(),
      server: z.string(),
      topic: z.string(),
      username: z.string(),
      password: z.string(),
      priority: z.string(),
      tags: z.string(),
      title_template: z.string(),
      body_template: z.string(),
      url: z.string(),
      method: z.string(),
      body: z.string(),
      headers: z.array(z.object({ name: z.string(), value: z.string() })),
    })
    .superRefine((values, ctx) => {
      // Header names are a token grammar, and the ones the notifier sets itself
      // cannot be overridden from here - failing the row beats failing the send.
      const reserved = RESERVED_HEADERS[values.type]
      const seen = new Set<string>()
      values.headers.forEach((header, index) => {
        const name = header.name.trim()
        if (!name) return
        const issue = (message: string) =>
          ctx.addIssue({
            code: 'custom',
            path: ['headers', index, 'name'],
            message,
          })
        if (!/^[\w!#$%&'*+.^`|~-]+$/.test(name)) return issue(t('notifications.headerNameInvalid'))
        const lower = name.toLowerCase()
        if (reserved.includes(lower)) return issue(t('notifications.headerReserved'))
        if (seen.has(lower)) return issue(t('notifications.headerDuplicate'))
        seen.add(lower)
      })
    })
}
// Headers the notifier (or the transport) owns, so a custom entry cannot quietly
// change the priority or replace the credentials.
const RESERVED_HEADERS: Record<string, string[]> = {
  ntfy: ['authorization', 'content-type', 'title', 'priority', 'tags'],
  webhook: ['authorization', 'content-type', 'host', 'content-length'],
}

type FormValues = z.infer<ReturnType<typeof channelSchema>>

const EMPTY: FormValues = {
  name: '',
  type: 'ntfy',
  enabled: true,
  server: '',
  topic: '',
  username: '',
  password: '',
  priority: '3',
  tags: '',
  title_template: '',
  body_template: '',
  url: '',
  method: 'POST',
  body: '',
  headers: [],
}

function valuesFor(channel: NotifyChannel | null): FormValues {
  if (!channel) return EMPTY
  if (channel.type === 'ntfy') {
    const config = ntfyOf(channel)
    return {
      ...EMPTY,
      name: channel.name,
      type: 'ntfy',
      enabled: channel.enabled,
      server: config.server ?? '',
      topic: config.topic ?? '',
      username: config.username ?? '',
      priority: String(config.priority ?? 3),
      tags: config.tags ?? '',
      title_template: config.title_template ?? '',
      body_template: config.body_template ?? '',
      headers: headersOf(channel),
    }
  }
  const config = webhookOf(channel)
  return {
    ...EMPTY,
    name: channel.name,
    type: 'webhook',
    enabled: channel.enabled,
    url: config.url ?? '',
    method: config.method ?? 'POST',
    body: config.body ?? '',
    headers: headersOf(channel),
  }
}

type ChannelDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The channel being edited, or null when a new one is being added. */
  channel: NotifyChannel | null
  saving: boolean
  onSubmit: (input: ChannelInput) => Promise<void>
}

/**
 * Where a change is announced.
 *
 * The test button is not here: it belongs to the row, because what needs proving
 * is that a saved channel works, and a dialog closes before it can be answered.
 */
export function ChannelDialog({
  open,
  onOpenChange,
  channel,
  saving,
  onSubmit,
}: ChannelDialogProps) {
  const { t } = useTranslation()
  const form = useForm<FormValues>({
    resolver: zodResolver(channelSchema(t)),
    defaultValues: EMPTY,
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(channel))
  }, [open, channel, form])

  const type = form.watch('type')

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'headers',
  })

  function submit(values: FormValues) {
    // Blank names are the empty rows someone never filled in; everything else
    // goes out as written, minus the trailing whitespace.
    const headers = Object.fromEntries(
      values.headers
        .map((header) => ({ ...header, name: header.name.trim() }))
        .filter((header) => header.name !== '')
        .map((header) => [header.name, header.value])
    )
    const config =
      values.type === 'ntfy'
        ? {
            server: values.server.trim(),
            topic: values.topic.trim(),
            priority: Number(values.priority),
            ...(values.username.trim() ? { username: values.username.trim() } : {}),
            // A blank password on an existing channel means keep the stored one.
            ...(values.password ? { password: values.password } : {}),
            ...(values.tags.trim() ? { tags: values.tags.trim() } : {}),
            ...(values.title_template.trim()
              ? { title_template: values.title_template.trim() }
              : {}),
            ...(values.body_template.trim() ? { body_template: values.body_template.trim() } : {}),
            ...(Object.keys(headers).length ? { headers } : {}),
          }
        : {
            url: values.url.trim(),
            method: values.method,
            ...(values.body.trim() ? { body: values.body.trim() } : {}),
            ...(Object.keys(headers).length ? { headers } : {}),
          }

    void onSubmit({
      name: values.name,
      type: values.type,
      enabled: values.enabled,
      config,
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full gap-0 p-0 sm:max-w-xl'>
        <SheetHeader className='border-b pe-10'>
          <SheetTitle>
            {channel ? t('notifications.editTitle') : t('notifications.addTitle')}
          </SheetTitle>
          <SheetDescription>{t('notifications.description')}</SheetDescription>
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
                        <Input placeholder={t('notifications.namePlaceholder')} {...field} />
                      </FormControl>
                      <FormDescription>{t('notifications.nameHint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='type'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('notifications.typeColumn')}</FormLabel>
                      <Select name={field.name} value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {CHANNEL_TYPES.map((option) => (
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
              </div>

              {type === 'ntfy' ? (
                <>
                  <div className='grid items-start gap-4 sm:grid-cols-2'>
                    <FormField
                      control={form.control}
                      name='server'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.serverLabel')}</FormLabel>
                          <FormControl>
                            <Input placeholder={t('notifications.serverPlaceholder')} {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name='topic'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.topicLabel')}</FormLabel>
                          <FormControl>
                            <Input placeholder={t('notifications.topicPlaceholder')} {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <div className='grid items-start gap-4 sm:grid-cols-2'>
                    <FormField
                      control={form.control}
                      name='tags'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.tagsLabel')}</FormLabel>
                          <FormControl>
                            <Input placeholder={t('notifications.tagsPlaceholder')} {...field} />
                          </FormControl>
                          <FormDescription>{t('notifications.tagsHint')}</FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name='priority'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.priorityLabel')}</FormLabel>
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
                              {PRIORITY_VALUES.map((value) => (
                                <SelectItem key={value} value={value}>
                                  {t(`notifications.priority${value}`)}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          <FormDescription>{t('notifications.priorityHint')}</FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <div className='grid items-start gap-4 sm:grid-cols-2'>
                    <FormField
                      control={form.control}
                      name='username'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.usernameLabel')}</FormLabel>
                          <FormControl>
                            <Input
                              placeholder={t('notifications.optionalPlaceholder')}
                              {...field}
                            />
                          </FormControl>
                          <FormDescription>{t('notifications.usernameHint')}</FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name='password'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.passwordLabel')}</FormLabel>
                          <FormControl>
                            <Input
                              type='password'
                              autoComplete='new-password'
                              placeholder={
                                channel
                                  ? t('notifications.passwordKeep')
                                  : t('notifications.optionalPlaceholder')
                              }
                              {...field}
                            />
                          </FormControl>
                          <FormDescription>{t('notifications.passwordHint')}</FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <FormField
                    control={form.control}
                    name='title_template'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('notifications.titleTemplateLabel')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder='{{.SubDomain}}.{{.Domain}} {{.Status}}'
                            {...field}
                            className='font-mono text-xs'
                          />
                        </FormControl>
                        <FormDescription>{t('notifications.titleTemplateHint')}</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='body_template'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('notifications.messageTemplateLabel')}</FormLabel>
                        <FormControl>
                          <Textarea
                            rows={4}
                            placeholder={'{{.SubDomain}}.{{.Domain}} is now {{.NewIP}}'}
                            {...field}
                            className='font-mono text-xs'
                          />
                        </FormControl>
                        <FormDescription>{t('notifications.messageTemplateHint')}</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              ) : (
                <>
                  <div className='grid items-start gap-4 sm:grid-cols-[1fr_9rem]'>
                    <FormField
                      control={form.control}
                      name='url'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.addressLabel')}</FormLabel>
                          <FormControl>
                            <Input placeholder={t('notifications.addressPlaceholder')} {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name='method'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('notifications.methodLabel')}</FormLabel>
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
                              {METHODS.map((method) => (
                                <SelectItem key={method} value={method}>
                                  {method}
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
                    name='body'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('notifications.bodyLabel')}</FormLabel>
                        <FormControl>
                          <Textarea rows={6} {...field} className='font-mono text-xs' />
                        </FormControl>
                        <FormDescription>
                          {t('notifications.bodyHint')} <code>{'{{.Domain}} is {{.Status}}'}</code>.
                          {t('notifications.defaultJsonHint')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              )}

              {/* Both channel types take extra headers, so the editor is shared.
                  The headers the notifier sets itself are not editable here. */}
              <div className='space-y-3'>
                <div className='space-y-0.5'>
                  <h4 className='text-sm font-medium'>{t('notifications.headersTitle')}</h4>
                  <p className='text-muted-foreground text-sm'>
                    {type === 'ntfy'
                      ? t('notifications.headersHintNtfy')
                      : t('notifications.headersHintWebhook')}
                  </p>
                </div>

                {fields.map((field, index) => (
                  <div key={field.id} className='flex items-start gap-2'>
                    <FormField
                      control={form.control}
                      name={`headers.${index}.name`}
                      render={({ field }) => (
                        <FormItem className='flex-1'>
                          <FormControl>
                            <Input
                              placeholder={t('notifications.headerNamePlaceholder')}
                              aria-label={t('ui.headerName', { index: index + 1 })}
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name={`headers.${index}.value`}
                      render={({ field }) => (
                        <FormItem className='flex-1'>
                          <FormControl>
                            <Input
                              placeholder={t('notifications.headerValuePlaceholder')}
                              aria-label={t('ui.headerValue', { index: index + 1 })}
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      className='text-muted-foreground size-9 shrink-0'
                      onClick={() => remove(index)}
                    >
                      <Trash2 />
                      <span className='sr-only'>{t('notifications.removeHeader')}</span>
                    </Button>
                  </div>
                ))}

                <FormField
                  control={form.control}
                  name='headers'
                  render={() => (
                    <FormItem>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => append({ name: '', value: '' })}
                >
                  <Plus />
                  {t('notifications.addHeader')}
                </Button>
              </div>

              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                    <div className='space-y-0.5'>
                      <FormLabel>{t('records.enabledLabel')}</FormLabel>
                      <FormDescription>{t('notifications.enabledHint')}</FormDescription>
                    </div>
                    <FormControl>
                      <Switch checked={field.value} onCheckedChange={field.onChange} />
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
                {channel ? t('common.saveChanges') : t('notifications.add')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
