import { createContext, useContext, useEffect, useState } from 'react'
import { fonts } from '@/config/fonts'
import { readStored, removeStored, subscribeStored, writeStored } from '@/lib/storage'

type Font = (typeof fonts)[number]

const FONT_STORAGE_KEY = 'font'

type FontContextType = {
  font: Font
  setFont: (font: Font) => void
  resetFont: () => void
}

const FontContext = createContext<FontContextType | null>(null)

export function FontProvider({ children }: { children: React.ReactNode }) {
  const [font, _setFont] = useState<Font>(() => {
    const savedFont = readStored(FONT_STORAGE_KEY)
    return fonts.includes(savedFont as Font) ? (savedFont as Font) : fonts[0]
  })

  // Another tab's choice reaches this one without a reload.
  useEffect(
    () =>
      subscribeStored(FONT_STORAGE_KEY, (value) =>
        _setFont(fonts.includes(value as Font) ? (value as Font) : fonts[0])
      ),
    []
  )

  useEffect(() => {
    const applyFont = (font: string) => {
      const root = document.documentElement
      root.classList.forEach((cls) => {
        if (cls.startsWith('font-')) root.classList.remove(cls)
      })
      root.classList.add(`font-${font}`)
    }

    applyFont(font)
  }, [font])

  const setFont = (font: Font) => {
    writeStored(FONT_STORAGE_KEY, font)
    _setFont(font)
  }

  const resetFont = () => {
    removeStored(FONT_STORAGE_KEY)
    _setFont(fonts[0])
  }

  return <FontContext value={{ font, setFont, resetFont }}>{children}</FontContext>
}

// eslint-disable-next-line react-refresh/only-export-components
export const useFont = () => {
  const context = useContext(FontContext)
  if (!context) {
    throw new Error('useFont must be used within a FontProvider')
  }
  return context
}
