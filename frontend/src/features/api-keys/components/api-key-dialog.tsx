import { useTranslation } from 'react-i18next'
import { type TFunction } from 'i18next'
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { listAccounts } from '@/features/accounts/api'
import { API_KEY_ANY_DOMAIN, type APIKeyInput } from '@/features/api-keys/api'
import { listDomains } from '@/features/domains/api'
import { type APIKey } from '@/lib/types'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { Switch } from '@/components/ui/switch'

/** Built per render so the validation messages are translated too. */
function keySchema(t: TFunction) {
  return z.object({
    name: z.string().trim().min(1, t('apiKeys.nameRequired')),
    domain: z.string().trim().min(1, t('apiKeys.domainRequired')),
    account_id: z.number().int().positive(t('apiKeys.accountRequired')),
    enabled: z.boolean(),
  })
}

type FormValues = z.infer<ReturnType<typeof keySchema>>

const EMPTY: FormValues = {
  name: '',
  domain: API_KEY_ANY_DOMAIN,
  account_id: 0,
  enabled: true,
}

function valuesFor(apiKey: APIKey | null): FormValues {
  if (!apiKey) return EMPTY
  return {
    name: apiKey.name,
    domain: apiKey.domain,
    account_id: apiKey.account_id,
    enabled: apiKey.enabled,
  }
}

type APIKeyDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The key being edited, or null when a new one is being issued. */
  apiKey: APIKey | null
  saving: boolean
  onSubmit: (values: APIKeyInput) => Promise<void>
}

/**
 * A credential for the public REST API.
 *
 * The scope is one domain, or every one of them: a key that can reach something
 * it was never needed for is how a leaked script becomes a bigger problem than it
 * has to be.
 */
export function APIKeyDialog({ open, onOpenChange, apiKey, saving, onSubmit }: APIKeyDialogProps) {
  const { t } = useTranslation()
  const form = useForm<FormValues>({
    resolver: zodResolver(keySchema(t)),
    defaultValues: EMPTY,
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(apiKey))
  }, [open, apiKey, form])

  const accountId = form.watch('account_id')

  const accounts = useQuery({
    queryKey: ['accounts'],
    queryFn: () => listAccounts(),
    enabled: open,
  })
  // Read for the account the key would work through: the pair is what the API
  // stores, and a domain of another account can never be reached by this key.
  const domains = useQuery({
    queryKey: ['domains', accountId],
    queryFn: () => listDomains(accountId || undefined),
    enabled: open && accountId > 0,
  })

  function submit(values: FormValues) {
    void onSubmit({
      name: values.name,
      domain: values.domain,
      account_id: values.account_id,
      enabled: values.enabled,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{apiKey ? t('apiKeys.editTitle') : t('apiKeys.addTitle')}</DialogTitle>
          <DialogDescription>{t('apiKeys.dialogHint')}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} className='space-y-6'>
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('apiKeys.nameLabel')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('apiKeys.namePlaceholder')} {...field} />
                  </FormControl>
                  <FormDescription>{t('apiKeys.nameHint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='domain'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('apiKeys.domainLabel')}</FormLabel>
                  <Select name={field.name} value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue placeholder={t('apiKeys.domainPlaceholder')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value={API_KEY_ANY_DOMAIN}>{t('apiKeys.everyDomain')}</SelectItem>
                      {(domains.data ?? []).map((domain) => (
                        <SelectItem key={domain.name} value={domain.name}>
                          {domain.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {field.value === API_KEY_ANY_DOMAIN
                      ? 'Every domain of that account can be read or changed.'
                      : "Only this domain's records can be read or changed."}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='account_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('apiKeys.accountLabel')}</FormLabel>
                  <Select
                    name={field.name}
                    value={field.value ? String(field.value) : ''}
                    onValueChange={(value) => {
                      field.onChange(Number(value))
                      // A named scope has to be a domain of that account, so
                      // the one picked for the previous account is dropped; the
                      // wildcard covers any account and stays.
                      if (form.getValues('domain') !== API_KEY_ANY_DOMAIN) {
                        form.setValue('domain', '')
                      }
                    }}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue placeholder={t('apiKeys.accountPlaceholder')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {(accounts.data ?? []).map((account) => (
                        <SelectItem key={account.id} value={String(account.id)}>
                          {account.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>{t('apiKeys.accountHint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('apiKeys.enabledLabel')}</FormLabel>
                    <FormDescription>{t('apiKeys.enabledHint')}</FormDescription>
                  </div>
                  <FormControl>
                    <Switch checked={field.value} onCheckedChange={field.onChange} />
                  </FormControl>
                </FormItem>
              )}
            />

            <DialogFooter>
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
                {apiKey ? t('common.saveChanges') : t('apiKeys.createKey')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
