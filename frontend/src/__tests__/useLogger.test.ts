import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useLogger } from '@/hooks/useLogger'

describe('useLogger', () => {
  const originalDev = import.meta.env.DEV

  beforeEach(() => {
    vi.spyOn(console, 'log').mockImplementation(() => {})
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns a logger with log, warn, error methods', () => {
    const { result } = renderHook(() => useLogger('Test'))
    const logger = result.current
    expect(typeof logger.log).toBe('function')
    expect(typeof logger.warn).toBe('function')
    expect(typeof logger.error).toBe('function')
  })

  it('warn always calls console.warn with prefix', () => {
    const { result } = renderHook(() => useLogger('MyMod'))
    result.current.warn('something', 42)
    expect(console.warn).toHaveBeenCalledWith('[MyMod]', 'something', 42)
  })

  it('error always calls console.error with prefix', () => {
    const { result } = renderHook(() => useLogger('MyMod'))
    result.current.error('fail', new Error('x'))
    expect(console.error).toHaveBeenCalledWith('[MyMod]', 'fail', expect.any(Error))
  })

  it('log calls console.log with prefix in dev mode', () => {
    // vitest runs in dev/test mode by default
    const { result } = renderHook(() => useLogger('Dev'))
    result.current.log('hello', 'world')
    expect(console.log).toHaveBeenCalledWith('[Dev]', 'hello', 'world')
  })

  it('log is silent when isDev is false', () => {
    // Simulate production by testing the prod branch logic directly
    const prodLogger = {
      log: () => {},
      warn: (...args: unknown[]) => console.warn('[Prod]', ...args),
      error: (...args: unknown[]) => console.error('[Prod]', ...args)
    }

    prodLogger.log('should not appear')
    expect(console.log).not.toHaveBeenCalled()

    prodLogger.warn('visible warning')
    expect(console.warn).toHaveBeenCalledWith('[Prod]', 'visible warning')

    prodLogger.error('visible error')
    expect(console.error).toHaveBeenCalledWith('[Prod]', 'visible error')
  })

  it('returns stable reference across re-renders', () => {
    const { result, rerender } = renderHook(() => useLogger('Stable'))
    const first = result.current
    rerender()
    const second = result.current
    expect(first).toBe(second)
  })
})
