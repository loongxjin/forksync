import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'

interface SettingsDrawerContextValue {
  open: boolean
  openDrawer: () => void
  closeDrawer: () => void
}

const SettingsDrawerContext = createContext<SettingsDrawerContextValue | null>(null)

export function SettingsDrawerProvider({ children }: { children: ReactNode }): JSX.Element {
  const [open, setOpen] = useState(false)

  const openDrawer = useCallback(() => setOpen(true), [])
  const closeDrawer = useCallback(() => setOpen(false), [])

  return (
    <SettingsDrawerContext.Provider value={{ open, openDrawer, closeDrawer }}>
      {children}
    </SettingsDrawerContext.Provider>
  )
}

export function useSettingsDrawer(): SettingsDrawerContextValue {
  const ctx = useContext(SettingsDrawerContext)
  if (!ctx) {
    throw new Error('useSettingsDrawer must be used within SettingsDrawerProvider')
  }
  return ctx
}
