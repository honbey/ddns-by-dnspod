import { useTranslation } from 'react-i18next'
import { Terminal } from 'lucide-react'
import { API_KEY_ANY_DOMAIN, publicApiBase } from '@/features/api-keys/api'
import { Paste } from '@/components/paste'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'

type APIKeyCreatedProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  domain: string
  secret: string
}

/**
 * The one time the plaintext exists.
 *
 * Only a prefix is stored, so this is not "here it is anyway" - once closed it
 * cannot be recovered, which is why it is shown before anything else can happen
 * and the example command carries it rather than leaving that to memory.
 */
export function APIKeyCreatedDialog({ open, onOpenChange, domain, secret }: APIKeyCreatedProps) {
  const { t } = useTranslation()
  // A wildcard key reaches every domain, so the sample names one of them
  const example = `curl -H "X-API-Key: ${secret}" ${publicApiBase()}/v1/domains/${
    domain === API_KEY_ANY_DOMAIN ? 'example.com' : domain
  }/records`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('apiKeys.createdTitle')}</DialogTitle>
          <DialogDescription>{t('apiKeys.copyNow')}</DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='space-y-2'>
            <Label>Key</Label>
            <div className='rounded-md border bg-muted/40 px-3 py-2'>
              {/* Wrapping, not truncating: a key you cannot read whole is a key
                  you cannot copy with confidence. */}
              <Paste value={secret} label={t('apiKeys.apiKeyLabel')} wrap />
            </div>
          </div>

          <div className='space-y-2'>
            <Label>{t('apiKeys.exampleLabel')}</Label>
            <div className='flex items-start gap-2 rounded-md border bg-muted/40 px-3 py-2'>
              <Terminal aria-hidden='true' className='mt-0.5 size-3.5 shrink-0' />
              <Paste value={example} label={t('apiKeys.exampleCommandLabel')} wrap />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button size='sm' onClick={() => onOpenChange(false)}>
            {t('common.done')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
