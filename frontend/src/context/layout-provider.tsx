import { createContext, useContext, useEffect, useState } from 'react'
import { readStored, subscribeStored, writeStored } from '@/lib/storage'

export type Collapsible = 'offcanvas' | 'icon' | 'none'
type Variant = 'inset' | 'sidebar' | 'floating'

// Keys the drawer and the sidebar read; the names are the ones the layout was
// shipped with, kept so a saved value keeps meaning what it meant.
const LAYOUT_COLLAPSIBLE_STORAGE_KEY = 'layout_collapsible'
const LAYOUT_VARIANT_STORAGE_KEY = 'layout_variant'

// Default values
const DEFAULT_VARIANT = 'inset'
const DEFAULT_COLLAPSIBLE = 'icon'

type LayoutContextType = {
  resetLayout: () => void

  defaultCollapsible: Collapsible
  collapsible: Collapsible
  setCollapsible: (collapsible: Collapsible) => void

  defaultVariant: Variant
  variant: Variant
  setVariant: (variant: Variant) => void
}

const LayoutContext = createContext<LayoutContextType | null>(null)

type LayoutProviderProps = {
  children: React.ReactNode
}

export function LayoutProvider({ children }: LayoutProviderProps) {
  const [collapsible, _setCollapsible] = useState<Collapsible>(() => {
    const saved = readStored(LAYOUT_COLLAPSIBLE_STORAGE_KEY)
    return (saved as Collapsible) || DEFAULT_COLLAPSIBLE
  })

  const [variant, _setVariant] = useState<Variant>(() => {
    const saved = readStored(LAYOUT_VARIANT_STORAGE_KEY)
    return (saved as Variant) || DEFAULT_VARIANT
  })

  // Both follow the tab that changed them.
  useEffect(
    () =>
      subscribeStored(LAYOUT_COLLAPSIBLE_STORAGE_KEY, (value) =>
        _setCollapsible((value as Collapsible) || DEFAULT_COLLAPSIBLE)
      ),
    []
  )

  useEffect(
    () =>
      subscribeStored(LAYOUT_VARIANT_STORAGE_KEY, (value) =>
        _setVariant((value as Variant) || DEFAULT_VARIANT)
      ),
    []
  )

  const setCollapsible = (newCollapsible: Collapsible) => {
    _setCollapsible(newCollapsible)
    writeStored(LAYOUT_COLLAPSIBLE_STORAGE_KEY, newCollapsible)
  }

  const setVariant = (newVariant: Variant) => {
    _setVariant(newVariant)
    writeStored(LAYOUT_VARIANT_STORAGE_KEY, newVariant)
  }

  const resetLayout = () => {
    setCollapsible(DEFAULT_COLLAPSIBLE)
    setVariant(DEFAULT_VARIANT)
  }

  const contextValue: LayoutContextType = {
    resetLayout,
    defaultCollapsible: DEFAULT_COLLAPSIBLE,
    collapsible,
    setCollapsible,
    defaultVariant: DEFAULT_VARIANT,
    variant,
    setVariant,
  }

  return <LayoutContext value={contextValue}>{children}</LayoutContext>
}

// Define the hook for the provider
// eslint-disable-next-line react-refresh/only-export-components
export function useLayout() {
  const context = useContext(LayoutContext)
  if (!context) {
    throw new Error('useLayout must be used within a LayoutProvider')
  }
  return context
}
