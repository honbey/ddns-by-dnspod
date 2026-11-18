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
export function HelpTip({
  field,
  children,
  className,
}: {
  /** What the hint explains, named in the button's label. */
  field: string
  children: ReactNode
  className?: string
}) {
  const { t } = useTranslation()
  return (
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
      {/* Left-aligned with the icon and kept off the viewport edge: inside a
          right-hand sheet a centred bubble reaches over the panel it explains. */}
      <TooltipContent
        className='max-w-xs'
        side='top'
        align='start'
        sideOffset={6}
        collisionPadding={12}
      >
        {children}
      </TooltipContent>
    </Tooltip>
  )
}

/**
 * The same button for a field that belongs to a form: it also fills the
 * description slot, so the hint stays announced on the control itself rather
 * than only behind the help button.
 */
export function FieldHelp({
  field,
  children,
  className,
  announce = true,
}: {
  field: string
  children: ReactNode
  className?: string
  /**
   * False when the field also renders a FormDescription: that element already
   * carries the description id, and a second one with the same id would make
   * the control's aria-describedby resolve to two elements.
   */
  announce?: boolean
}) {
  const { formDescriptionId } = useFormField()
  return (
    <>
      <HelpTip field={field} className={className}>
        {children}
      </HelpTip>
      {announce ? (
        <span id={formDescriptionId} className='sr-only'>
          {children}
        </span>
      ) : null}
    </>
  )
}
