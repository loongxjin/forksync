import * as React from 'react'
import { cn } from '@/lib/utils'

interface SheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
  side?: 'left' | 'right' | 'top' | 'bottom'
  className?: string
}

const SheetContext = React.createContext<{
  open: boolean
  onOpenChange: (open: boolean) => void
} | null>(null)

function useSheet() {
  const ctx = React.useContext(SheetContext)
  if (!ctx) throw new Error('Sheet components must be used inside <Sheet>')
  return ctx
}

export function Sheet({ open, onOpenChange, children }: SheetProps): JSX.Element {
  return (
    <SheetContext.Provider value={{ open, onOpenChange }}>
      {children}
    </SheetContext.Provider>
  )
}

export function SheetTrigger({
  children,
  asChild
}: {
  children: React.ReactNode
  asChild?: boolean
}): JSX.Element {
  const { onOpenChange } = useSheet()

  if (asChild && React.isValidElement(children)) {
    return React.cloneElement(children, {
      onClick: (e: React.MouseEvent) => {
        onOpenChange(true)
        children.props.onClick?.(e)
      }
    } as React.HTMLAttributes<HTMLElement>)
  }

  return (
    <button onClick={() => onOpenChange(true)}>
      {children}
    </button>
  )
}

export function SheetContent({
  children,
  side = 'right',
  className
}: {
  children: React.ReactNode
  side?: 'left' | 'right' | 'top' | 'bottom'
  className?: string
}): JSX.Element {
  const { open, onOpenChange } = useSheet()
  const contentRef = React.useRef<HTMLDivElement>(null)

  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onOpenChange(false)
    }
    if (open) {
      document.addEventListener('keydown', handleKeyDown)
      document.body.style.overflow = 'hidden'
    }
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.body.style.overflow = ''
    }
  }, [open, onOpenChange])

  // Focus trap — Tab cycles inside the sheet drawer, not into the background.
  React.useEffect(() => {
    if (!open || !contentRef.current) return
    const el = contentRef.current
    const prevFocus = document.activeElement as HTMLElement | null
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
      prevFocus?.focus()
    }
  }, [open])

  const sideClasses = {
    // left/right panels sit BELOW the title bar (logo + window controls),
    // so they no longer cover it when slid open. Height derives from
    // top/bottom instead of h-full.
    left: 'left-0 w-80',
    right: 'right-0 w-80',
    top: 'inset-x-0 top-0 w-full h-auto',
    bottom: 'inset-x-0 bottom-0 w-full h-auto'
  }

  const closedTranslate = {
    left: '-translate-x-full',
    right: 'translate-x-full',
    top: '-translate-y-full',
    bottom: 'translate-y-full'
  }

  // The overlay container starts below the title bar so the panel content
  // doesn't cover the logo + window controls. But the BACKDROP itself covers
  // the full viewport (including the title bar) for a consistent blur effect.
  return (
    <>
      {/* Backdrop — full viewport including title bar, but below the title bar */}
      {open && (
        <div
          className={cn(
            'fixed inset-0 z-40 bg-black/40 backdrop-blur-sm transition-opacity duration-300',
            open ? 'opacity-100' : 'opacity-0'
          )}
          onClick={() => onOpenChange(false)}
        />
      )}
      {/* Content container — below title bar */}
      <div
        className={cn(
          'fixed top-[var(--titlebar-height)] inset-x-0 bottom-0 z-50 transition-[visibility] duration-300',
          'pointer-events-none',
          !open && 'invisible'
        )}
      >
      {/* Content */}
      <div
        ref={contentRef}
        role="dialog"
        aria-modal="true"
        data-state={open ? 'open' : 'closed'}
        className={cn(
          'fixed top-[var(--titlebar-height)] bottom-0 z-50 bg-card shadow-2xl border-border/50',
          'pointer-events-auto',
          'transition-transform duration-300 ease-in-out',
          sideClasses[side],
          open ? 'translate-x-0 translate-y-0' : closedTranslate[side],
          className
        )}
      >
        {children}
      </div>
      </div>
    </>
  )
}

export function SheetHeader({
  children,
  className
}: {
  children: React.ReactNode
  className?: string
}): JSX.Element {
  return <div className={cn('px-6 py-5 border-b border-border', className)}>{children}</div>
}

export function SheetTitle({
  children,
  className
}: {
  children: React.ReactNode
  className?: string
}): JSX.Element {
  return <h2 className={cn('text-lg font-semibold', className)}>{children}</h2>
}

export function SheetDescription({
  children,
  className
}: {
  children: React.ReactNode
  className?: string
}): JSX.Element {
  return <p className={cn('text-sm text-muted-foreground', className)}>{children}</p>
}

export function SheetFooter({
  children,
  className
}: {
  children: React.ReactNode
  className?: string
}): JSX.Element {
  return <div className={cn('px-6 py-4 border-t border-border', className)}>{children}</div>
}

export function SheetClose({
  children,
  asChild
}: {
  children: React.ReactNode
  asChild?: boolean
}): JSX.Element {
  const { onOpenChange } = useSheet()

  if (asChild && React.isValidElement(children)) {
    return React.cloneElement(children, {
      onClick: (e: React.MouseEvent) => {
        onOpenChange(false)
        children.props.onClick?.(e)
      }
    } as React.HTMLAttributes<HTMLElement>)
  }

  return (
    <button onClick={() => onOpenChange(false)}>
      {children}
    </button>
  )
}
