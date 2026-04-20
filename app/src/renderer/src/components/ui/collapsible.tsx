import * as React from 'react'
import { cn } from '@/lib/utils'

interface CollapsibleProps {
  open: boolean
  children: React.ReactNode
  className?: string
}

const CollapsibleContext = React.createContext<{
  open: boolean
} | null>(null)

function useCollapsible() {
  const ctx = React.useContext(CollapsibleContext)
  if (!ctx) throw new Error('Collapsible components must be used inside <Collapsible>')
  return ctx
}

export function Collapsible({ open, children, className }: CollapsibleProps): JSX.Element {
  return (
    <CollapsibleContext.Provider value={{ open }}>
      <div className={cn('overflow-hidden transition-all duration-300 ease-in-out', className)}>
        {children}
      </div>
    </CollapsibleContext.Provider>
  )
}

export function CollapsibleContent({
  children,
  className
}: {
  children: React.ReactNode
  className?: string
}): JSX.Element {
  const { open } = useCollapsible()
  const contentRef = React.useRef<HTMLDivElement>(null)
  const [height, setHeight] = React.useState<number | undefined>(open ? undefined : 0)

  React.useEffect(() => {
    if (contentRef.current) {
      if (open) {
        // Measure after a frame to ensure layout is settled
        const raf = requestAnimationFrame(() => {
          if (contentRef.current) {
            setHeight(contentRef.current.scrollHeight)
            const timer = setTimeout(() => setHeight(undefined), 300)
            return () => clearTimeout(timer)
          }
        })
        return () => cancelAnimationFrame(raf)
      } else {
        setHeight(contentRef.current.scrollHeight)
        const raf = requestAnimationFrame(() => {
          setHeight(0)
        })
        return () => cancelAnimationFrame(raf)
      }
    }
  }, [open])

  return (
    <div
      className={cn('overflow-hidden transition-[height] duration-300 ease-in-out', className)}
      style={{ height }}
    >
      <div ref={contentRef}>{children}</div>
    </div>
  )
}
