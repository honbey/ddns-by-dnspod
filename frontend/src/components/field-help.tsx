import { type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleHelp } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useFormField } from '@/components/ui/form'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

/**
 * A hint behind an icon rather than under the field: the form keeps its shape,
 * and the explanation is there for whoever asks for it.
 *
 * The button is next to the label, never inside it: a label that contains a
 * button is announced as one control, and its text is read twice.
 */
export function FieldHelp({
  field,
  children,
  className,
}: {
  field: string
  children: ReactNode
  className?: string
}) {
  const { t } = useTranslation()
  const { formDescriptionId } = useFormField()
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type='button'
            aria-label={t('common.fieldHelp', { field })}
            className={cn(
              'text-muted-foreground shrink-0 rounded-sm focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none',
              className
            )}
          >
            <CircleHelp aria-hidden='true' className='size-3.5' />
          </button>
        </TooltipTrigger>
        <TooltipContent className='max-w-xs'>{children}</TooltipContent>
      </Tooltip>
      {/* The control's aria-describedby still names the description slot; with
          the visible text gone, this keeps the hint announced on the field
          itself rather than only behind the help button. */}
      <span id={formDescriptionId} className='sr-only'>
        {children}
      </span>
    </>
  )
}
