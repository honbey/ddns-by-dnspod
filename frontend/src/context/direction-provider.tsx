import { createContext, useContext, useEffect, useState } from 'react'
import { DirectionProvider as RdxDirProvider } from '@radix-ui/react-direction'
import { readStored, removeStored, subscribeStored, writeStored } from '@/lib/storage'

export type Direction = 'ltr' | 'rtl'

const DEFAULT_DIRECTION = 'ltr'
const DIRECTION_STORAGE_KEY = 'dir'

type DirectionContextType = {
  defaultDir: Direction
  dir: Direction
  setDir: (dir: Direction) => void
  resetDir: () => void
}

const DirectionContext = createContext<DirectionContextType | null>(null)

export function DirectionProvider({ children }: { children: React.ReactNode }) {
  const [dir, _setDir] = useState<Direction>(
    () => (readStored(DIRECTION_STORAGE_KEY) as Direction | null) || DEFAULT_DIRECTION
  )

  useEffect(() => {
    const htmlElement = document.documentElement
    htmlElement.setAttribute('dir', dir)
  }, [dir])

  // Another tab flipping direction flips this one with it.
  useEffect(
    () =>
      subscribeStored(DIRECTION_STORAGE_KEY, (value) =>
        _setDir((value as Direction | null) ?? DEFAULT_DIRECTION)
      ),
    []
  )

  const setDir = (dir: Direction) => {
    _setDir(dir)
    writeStored(DIRECTION_STORAGE_KEY, dir)
  }

  const resetDir = () => {
    _setDir(DEFAULT_DIRECTION)
    removeStored(DIRECTION_STORAGE_KEY)
  }

  return (
    <DirectionContext
      value={{
        defaultDir: DEFAULT_DIRECTION,
        dir,
        setDir,
        resetDir,
      }}
    >
      <RdxDirProvider dir={dir}>{children}</RdxDirProvider>
    </DirectionContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useDirection() {
  const context = useContext(DirectionContext)
  if (!context) {
    throw new Error('useDirection must be used within a DirectionProvider')
  }
  return context
}
