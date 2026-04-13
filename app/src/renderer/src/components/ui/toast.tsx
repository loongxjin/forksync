/**
 * Simple Toast notification component
 */

import { useEffect, type ReactNode } from 'react'

interface ToastProps {
  message: string
  visible: boolean
  onClose: () => void
  duration?: number
  type?: 'info' | 'success' | 'warning' | 'error'
}

export function Toast({
  message,
  visible,
  onClose,
  duration = 2000,
  type = 'info'
}: ToastProps): JSX.Element | null {
  useEffect(() => {
    if (visible && duration > 0) {
      const timer = setTimeout(onClose, duration)
      return () => clearTimeout(timer)
    }
  }, [visible, duration, onClose])

  if (!visible) return null

  const bgColors: Record<string, string> = {
    info: 'bg-blue-500',
    success: 'bg-green-500',
    warning: 'bg-yellow-500',
    error: 'bg-red-500'
  }

  return (
    <div
      className={`fixed left-1/2 top-4 z-50 -translate-x-1/2 transform rounded-md px-4 py-2 text-sm font-medium text-white shadow-lg transition-all ${bgColors[type]}`}
    >
      {message}
    </div>
  )
}

// Simple hook-like toast state management
import { useState, useCallback } from 'react'

export interface ToastState {
  message: string
  visible: boolean
  type: 'info' | 'success' | 'warning' | 'error'
}

export function useToast() {
  const [toast, setToast] = useState<ToastState>({
    message: '',
    visible: false,
    type: 'info'
  })

  const showToast = useCallback(
    (message: string, type: ToastState['type'] = 'info', duration = 2000) => {
      setToast({ message, visible: true, type })
      setTimeout(() => {
        setToast((prev) => ({ ...prev, visible: false }))
      }, duration)
    },
    []
  )

  const hideToast = useCallback(() => {
    setToast((prev) => ({ ...prev, visible: false }))
  }, [])

  return { toast, showToast, hideToast }
}
