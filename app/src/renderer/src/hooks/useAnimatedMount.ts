import { useState, useEffect } from 'react'

/**
 * Manages mounted state with delayed unmount for CSS exit animations.
 * When `open` becomes true, mounts immediately.
 * When `open` becomes false, waits `delayMs` before unmounting to allow exit animation.
 */
export function useAnimatedMount(open: boolean, delayMs = 200): boolean {
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    if (open) {
      setMounted(true)
    } else if (mounted) {
      const timer = setTimeout(() => setMounted(false), delayMs)
      return () => clearTimeout(timer)
    }
  }, [open, mounted, delayMs])

  return mounted
}
