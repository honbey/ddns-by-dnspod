import { useEffect, useRef, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useTranslation } from 'react-i18next'

type PasteProps = {
  value: string
  /** What is being copied, used only for the confirmation. */
  label?: string
  /**
   * Let the value wrap instead of truncating. Off for table cells and hero
   * displays where one line is the design; on inside dialogs, where losing the
   * middle of a key to truncation defeats the purpose of showing it.
   */
  wrap?: boolean
  className?: string
}

/**
 * A value someone will paste into a terminal, with the one affordance that
 * saves retyping it.
 *
 * The clipboard refuses outside a secure context, so a failure says the value
 * is still selectable rather than pretending it worked.
 */
export function Paste({ value, label, wrap, className }: PasteProps) {
  const { t } = useTranslation()
  const name = label ?? t('common.value')
  const [copied, setCopied] = useState(false)
  const resetTimer = useRef<number>(0)

  // The timer has to outlive nothing: it used to fire after the row was gone,
  // and a second copy's check mark was switched off by the first copy's timer.
  useEffect(() => () => window.clearTimeout(resetTimer.current), [])

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      window.clearTimeout(resetTimer.current)
      setCopied(true)
      resetTimer.current = window.setTimeout(() => setCopied(false), 1500)
      toast.success(t('common.copied', { label: name }))
    } catch {
      toast.error(t('common.clipboardRefused'))
    }
  }

  return (
    <button
      type='button'
      onClick={copy}
      className={cn(
        'group flex w-full items-center gap-2 text-start',
        'cursor-pointer rounded-sm focus-visible:outline-none',
        className
      )}
      aria-label={t('common.copyLabel', { label: name })}
    >
      <code
        className={cn(
          'font-mono text-sm',
          wrap ? 'break-all whitespace-break-spaces' : 'truncate text-base'
        )}
      >
        {value}
      </code>
      {copied ? (
        <Check aria-hidden='true' className='size-3.5 shrink-0' />
      ) : (
        <Copy
          aria-hidden='true'
          className='text-muted-foreground size-3.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100'
        />
      )}
    </button>
  )
}
