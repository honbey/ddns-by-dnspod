import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * What a failed query looks like: the reason and the way out, in one place so
 * two pages cannot describe the same failure differently.
 */
export function ErrorState({ onRetry, className }: { onRetry: () => void; className?: string }) {
  const { t } = useTranslation()

  // Announced, since the region appears after the failure. The toast says the
  // same thing, so a reader hears it twice - cheaper than a message that fades.
  return (
    <div role='alert' className={cn('flex flex-col items-start gap-2 py-3 text-sm', className)}>
      <p className='text-muted-foreground'>{t('common.somethingWentWrong')}</p>
      <Button variant='outline' size='sm' onClick={onRetry}>
        {t('common.retry')}
      </Button>
    </div>
  )
}
