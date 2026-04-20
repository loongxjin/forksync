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

  const sideClasses = {
    left: 'inset-y-0 left-0 h-full w-80',
    right: 'inset-y-0 right-0 h-full w-80',
    top: 'inset-x-0 top-0 w-full h-auto',
    bottom: 'inset-x-0 bottom-0 w-full h-auto'
  }

  const closedTranslate = {
    left: '-translate-x-full',
    right: 'translate-x-full',
    top: '-translate-y-full',
    bottom: 'translate-y-full'
  }

  // When closed, render invisible to allow exit transition; after transition, hidden via pointer-events
  return (
    <div
      className={cn(
        'fixed inset-0 z-50 transition-[visibility] duration-300',
        !open && 'invisible'
      )}
    >
      {/* Backdrop */}
      <div
        className={cn(
          'absolute inset-0 bg-black/40 backdrop-blur-sm transition-opacity duration-300',
          open ? 'opacity-100' : 'opacity-0'
        )}
        onClick={() => onOpenChange(false)}
      />
      {/* Content */}
      <div
        ref={contentRef}
        data-state={open ? 'open' : 'closed'}
        className={cn(
          'fixed z-50 bg-card shadow-2xl border-border/50',
          'transition-transform duration-300 ease-in-out',
          sideClasses[side],
          open ? 'translate-x-0 translate-y-0' : closedTranslate[side],
          className
        )}
      >
        {children}
      </div>
    </div>
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
