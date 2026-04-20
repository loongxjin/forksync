/**
 * Toast notification component with icons, animations, and close button
 */

import { useEffect, useState } from 'react'
import { Info, CheckCircle2, AlertTriangle, XCircle, X } from 'lucide-react'

interface ToastProps {
  message: string
  visible: boolean
  onClose: () => void
  duration?: number
  type?: 'info' | 'success' | 'warning' | 'error'
}

const TOAST_CONFIG: Record<string, {
  icon: React.ReactNode
  barColor: string
}> = {
  info: {
    icon: <Info size={16} className="text-primary" />,
    barColor: 'bg-primary'
  },
  success: {
    icon: <CheckCircle2 size={16} className="text-success" />,
    barColor: 'bg-success'
  },
  warning: {
    icon: <AlertTriangle size={16} className="text-warning" />,
    barColor: 'bg-warning'
  },
  error: {
    icon: <XCircle size={16} className="text-error" />,
    barColor: 'bg-error'
  }
}

export function Toast({
  message,
  visible,
  onClose,
  duration = 2000,
  type = 'info'
}: ToastProps): JSX.Element | null {
  const [mounted, setMounted] = useState(false)
  const [exiting, setExiting] = useState(false)

  useEffect(() => {
    if (visible) {
      setMounted(true)
      setExiting(false)
    } else if (mounted) {
      setExiting(true)
      const timer = setTimeout(() => setMounted(false), 200)
      return () => clearTimeout(timer)
    }
  }, [visible, mounted])

  useEffect(() => {
    if (visible && duration > 0) {
      const timer = setTimeout(onClose, duration)
      return () => clearTimeout(timer)
    }
  }, [visible, duration, onClose])

  if (!mounted) return null

  const config = TOAST_CONFIG[type]

  return (
    <div
      className={`fixed left-1/2 top-4 z-[60] -translate-x-1/2 transition-all duration-200 ${
        exiting ? 'opacity-0 -translate-y-1' : 'opacity-0 translate-y-[-8px]'
      }`}
      style={!exiting ? { opacity: 1, transform: 'translateX(-50%) translateY(0)' } : undefined}
    >
      <div className="flex items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 shadow-lg min-w-[280px] max-w-[420px]">
        {/* Left color bar */}
        <div className={`w-[3px] self-stretch rounded-full ${config.barColor}`} />
        {/* Icon */}
        <span className="shrink-0">{config.icon}</span>
        {/* Message */}
        <p className="flex-1 text-sm text-foreground">{message}</p>
        {/* Close button */}
        <button
          onClick={onClose}
          className="shrink-0 rounded-md p-0.5 text-muted-foreground hover:text-foreground transition-colors"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  )
}

// Simple hook-like toast state management
import { useCallback } from 'react'

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
