import { useEffect, useRef } from 'react'
import { useRouterState } from '@tanstack/react-router'
import LoadingBar, { type LoadingBarRef } from 'react-top-loading-bar'

export function NavigationProgress() {
  const ref = useRef<LoadingBarRef>(null)
  const started = useRef(false)
  const state = useRouterState()

  useEffect(() => {
    if (state.status === 'pending') {
      started.current = true
      ref.current?.continuousStart()
      return
    }
    // Completing a bar that never started sweeps one across the top of the
    // first paint, which is what the idle status on mount used to do.
    if (started.current) {
      started.current = false
      ref.current?.complete()
    }
  }, [state.status])

  return <LoadingBar color='var(--muted-foreground)' ref={ref} shadow={true} height={2} />
}
