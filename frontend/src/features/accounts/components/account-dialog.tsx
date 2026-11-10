import { useEffect } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { type TFunction } from 'i18next'
import { Loader2 } from 'lucide-react'
import { type AccountInput } from '@/features/accounts/api'
import { type Account } from '@/lib/types'
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
import { Switch } from '@/components/ui/switch'

/**
 * Built per render so the validation messages are translated too: a schema
 * frozen at module load would keep reporting in whatever language was active
 * then.
 */
function accountSchema(t: TFunction) {
  return z.object({
    name: z.string().trim().min(1, t('accounts.nameRequired')),
    secret_id: z.string().trim().min(1, t('accounts.secretIdRequired')),
    // Blank while editing means "keep the stored one": the API never returns it.
    secret_key: z.string(),
    is_default: z.boolean(),
  })
}

type FormValues = z.infer<ReturnType<typeof accountSchema>>

const EMPTY: FormValues = {
  name: '',
  secret_id: '',
  secret_key: '',
  is_default: false,
}

function valuesFor(account: Account | null): FormValues {
  if (!account) return EMPTY
  return {
    name: account.name,
    secret_id: account.secret_id,
    secret_key: '',
    is_default: account.is_default,
  }
}

type AccountDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The account being edited, or null when a new one is being added. */
  account: Account | null
  saving: boolean
  onSubmit: (values: AccountInput) => Promise<void>
}

/**
 * DNSPod credentials, stored encrypted.
 *
 * Only ever write half of a pair here: the key being blank while editing is how
 * it says "leave the stored one alone", which is what makes it possible to rename
 * an account without knowing its key.
 */
export function AccountDialog({
  open,
  onOpenChange,
  account,
  saving,
  onSubmit,
}: AccountDialogProps) {
  const { t } = useTranslation()
  const form = useForm<FormValues>({
    resolver: zodResolver(accountSchema(t)),
    defaultValues: EMPTY,
  })

  useEffect(() => {
    if (open) form.reset(valuesFor(account))
  }, [open, account, form])

  function submit(values: FormValues) {
    void onSubmit({
      name: values.name,
      secret_id: values.secret_id,
      secret_key: values.secret_key,
      is_default: values.is_default,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{account ? t('accounts.editTitle') : t('accounts.addTitle')}</DialogTitle>
          <DialogDescription>{t('accounts.dialogHint')}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} className='space-y-6'>
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('common.name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('accounts.namePlaceholder')} {...field} />
                  </FormControl>
                  <FormDescription>{t('accounts.nameHint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='secret_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('accounts.secretIdLabel')}</FormLabel>
                  <FormControl>
                    <Input {...field} className='font-mono' />
                  </FormControl>
                  <FormDescription>{t('accounts.secretIdHint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='secret_key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('accounts.secretKeyLabel')}</FormLabel>
                  <FormControl>
                    <Input type='password' {...field} autoComplete='new-password' />
                  </FormControl>
                  <FormDescription>
                    {account ? t('accounts.secretKeyKeep') : t('accounts.secretKeyEncrypted')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='is_default'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-3 rounded-md border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('common.isDefault')}</FormLabel>
                    <FormDescription>{t('accounts.defaultHint')}</FormDescription>
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
                {account ? t('common.saveChanges') : t('accounts.addTitle')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
