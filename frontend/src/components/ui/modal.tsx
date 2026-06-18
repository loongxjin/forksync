import { ReactNode, useEffect, useRef, useCallback } from 'react'
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
 *
 * Accessibility: role="dialog", aria-modal="true", Escape-to-close, body
 * scroll lock, and a focus trap that keeps Tab cycling inside the modal.
 * Focus is returned to the triggering element on close.
 */
export function Modal({
  open,
  onClose,
  children,
  maxWidth = 'max-w-lg',
  scrollable = false
}: ModalProps): JSX.Element | null {
  const mounted = useAnimatedMount(open)
  const containerRef = useRef<HTMLDivElement>(null)

  // Escape handler
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  // Body scroll lock
  useEffect(() => {
    if (!open) {
      document.body.style.overflow = ''
      return
    }
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = '' }
  }, [open])

  // Focus trap + initial focus on open
  useEffect(() => {
    if (!open || !containerRef.current) return
    const el = containerRef.current
    const prevFocus = document.activeElement as HTMLElement | null
    // Focus the first focusable element inside the modal
    const focusable = el.querySelector<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
    )
    focusable?.focus()

    const trap = (e: KeyboardEvent): void => {
      if (e.key !== 'Tab') return
      const focusables = el.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      )
      if (focusables.length === 0) return
      const first = focusables[0]
      const last = focusables[focusables.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', trap)
    return () => {
      document.removeEventListener('keydown', trap)
      // Return focus to the trigger on close
      prevFocus?.focus()
    }
  }, [open])

  const handleClose = useCallback(() => {
    // Delay close slightly to avoid the backdrop getting a stray click after
    // the modal closes and mouse-up fires on the element underneath.
    setTimeout(() => onClose(), 0)
  }, [onClose])

  if (!mounted) return null

  return (
    <div
      ref={containerRef}
      role="dialog"
      aria-modal="true"
      className={`fixed inset-0 z-50 flex items-center justify-center transition-[visibility] duration-200 ${!open && 'invisible'}`}
    >
      {/* Backdrop */}
      <div
        className={`absolute inset-0 bg-black/50 backdrop-blur-sm transition-opacity duration-200 ${open ? 'opacity-100' : 'opacity-0'}`}
        onClick={handleClose}
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
