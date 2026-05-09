import { ReactNode } from 'react'
import { useAnimatedMount } from '@/hooks/useAnimatedMount'

interface ModalProps {
  open: boolean
  onClose: () => void
  children: ReactNode
  /** Max width class, e.g. 'max-w-lg' (default) or 'max-w-2xl' */
  maxWidth?: string
  /** Enable scrollable content with max height */
  scrollable?: boolean
}

/**
 * Shared modal wrapper with backdrop, blur, and enter/exit animations.
 * Uses useAnimatedMount for delayed unmount to allow CSS exit transitions.
 */
export function Modal({
  open,
  onClose,
  children,
  maxWidth = 'max-w-lg',
  scrollable = false
}: ModalProps): JSX.Element | null {
  const mounted = useAnimatedMount(open)

  if (!mounted) return null

  return (
    <div className={`fixed inset-0 z-50 flex items-center justify-center transition-[visibility] duration-200 ${!open && 'invisible'}`}>
      {/* Backdrop */}
      <div
        className={`absolute inset-0 bg-black/50 backdrop-blur-sm transition-opacity duration-200 ${open ? 'opacity-100' : 'opacity-0'}`}
        onClick={onClose}
      />
      {/* Content */}
      <div
        className={`relative z-10 w-full ${maxWidth} ${scrollable ? 'max-h-[90vh] overflow-y-auto' : ''} rounded-xl border border-border bg-card p-6 shadow-2xl transition-all duration-200 ${
          open ? 'opacity-100 scale-100' : 'opacity-0 scale-95'
        }`}
      >
        {children}
      </div>
    </div>
  )
}
