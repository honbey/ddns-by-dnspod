import { useState } from 'react'
import { Switch } from '@/components/ui/switch'

type RowSwitchProps = {
  /** The state on the server, which is the one that counts once nothing is in flight. */
  checked: boolean
  /** Resolves when the change has landed, so the switch knows it can let go. */
  onToggle: () => Promise<unknown>
  label: string
}

/**
 * A switch over one row, with its own idea of what is in flight.
 *
 * Reading the row alone was not enough: a second click before the refetch landed
 * was computed from the same row, so it asked for the state the first click had
 * already asked to change and was dropped without a word. This shows the answer
 * it is waiting for and holds still until that answer arrives.
 */
export function RowSwitch({ checked, onToggle, label }: RowSwitchProps) {
  const [pending, setPending] = useState<boolean | null>(null)
  const shown = pending ?? checked

  return (
    <Switch
      checked={shown}
      disabled={pending !== null}
      onCheckedChange={() => {
        setPending(!checked)
        // The failure is toasted by the global mutation handler; caught only so
        // the rejection is not left unhandled.
        void Promise.resolve(onToggle())
          .catch(() => {})
          .finally(() => setPending(null))
      }}
      aria-label={label}
    />
  )
}
