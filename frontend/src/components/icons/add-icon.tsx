import { type ComponentType, type SVGProps } from 'react'
import { Plus } from 'lucide-react'
import { cn } from '@/lib/utils'

type PlusPosition = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right'

type AddIconProps = {
  /** The glyph of the thing being added. */
  icon: ComponentType<SVGProps<SVGSVGElement>>
  /**
   * Where the plus sits. Each glyph leaves a different corner empty - the
   * account's key is bottom right, the certificate's ribbon is bottom centre -
   * so callers pick the corner that does not cover it. Physical corners, not
   * logical ones: the glyphs do not mirror in RTL, so start/end would move the
   * plus onto the key.
   */
  plusPosition?: PlusPosition
  className?: string
}

const PLUS_POSITION: Record<PlusPosition, string> = {
  'top-left': '-top-1 -left-1',
  'top-right': '-top-1 -right-1',
  'bottom-left': '-bottom-1 -left-1',
  'bottom-right': '-bottom-1 -right-1',
}

/**
 * The entity's own glyph with a small plus at one corner, for an "add" button.
 *
 * The plus is deliberately bare: a disc wide enough to hold it would hide more
 * of the glyph than the mark adds, and the corner is empty anyway.
 */
export function AddIcon({ icon: Icon, plusPosition = 'bottom-right', className }: AddIconProps) {
  return (
    <span
      aria-hidden='true'
      className={cn('relative inline-flex size-4 shrink-0 items-center justify-center', className)}
    >
      <Icon className='size-4' />
      <Plus className={cn('absolute size-2', PLUS_POSITION[plusPosition])} strokeWidth={4} />
    </span>
  )
}
