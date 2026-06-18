/**
 * ToastContext — global toast notification state
 *
 * Decouples toast infrastructure from RepoContext so any module can
 * trigger toasts without depending on repo state.
 */

import { createContext, useContext, type ReactNode } from 'react'
import { useToast } from '@/components/ui/toast'
import type { ToastState } from '@/components/ui/toast'

// ---------------------------------------------------------------------------
// Context value
// ---------------------------------------------------------------------------

interface ToastContextValue {
  toast: ToastState
  showToast: (message: string, type?: ToastState['type']) => void
  hideToast: () => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function ToastProvider({ children }: { children: ReactNode }): JSX.Element {
  const { toast, showToast, hideToast } = useToast()

  return (
    <ToastContext.Provider value={{ toast, showToast, hideToast }}>
      {children}
    </ToastContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// Consumer hook
// ---------------------------------------------------------------------------

export function useToastContext(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToastContext must be used within a ToastProvider')
  return ctx
}
