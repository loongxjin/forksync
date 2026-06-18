/**
 * useLogger — environment-aware logging hook for the renderer process.
 *
 * In development, all log levels are output to the console.
 * In production, only errors and warnings are output.
 *
 * Usage:
 *   const logger = useLogger('MyComponent')
 *   logger.log('something happened', data)
 *   logger.error('something failed', err)
 */

import { useRef } from 'react'

type LogLevel = 'log' | 'warn' | 'error'

interface Logger {
  log: (...args: unknown[]) => void
  warn: (...args: unknown[]) => void
  error: (...args: unknown[]) => void
}

const isDev = typeof import.meta !== 'undefined' && import.meta.env?.DEV

export function useLogger(prefix: string): Logger {
  const loggerRef = useRef<Logger>(
    isDev
      ? {
          log: (...args: unknown[]) => console.log(`[${prefix}]`, ...args),
          warn: (...args: unknown[]) => console.warn(`[${prefix}]`, ...args),
          error: (...args: unknown[]) => console.error(`[${prefix}]`, ...args)
        }
      : {
          log: () => {},
          warn: (...args: unknown[]) => console.warn(`[${prefix}]`, ...args),
          error: (...args: unknown[]) => console.error(`[${prefix}]`, ...args)
        }
  )
  return loggerRef.current
}
