import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDebouncedConfig, useDebouncedConfigMap } from '@/hooks/useDebouncedConfig'

describe('useDebouncedConfig', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('initializes with empty string when configValue is undefined', () => {
    const updateConfig = vi.fn()
    const { result } = renderHook(() =>
      useDebouncedConfig('sync.default_interval', undefined, updateConfig)
    )
    expect(result.current[0]).toBe('')
    expect(result.current[2]).toBe(false)
  })

  it('syncs local value from configValue on first render', () => {
    const updateConfig = vi.fn()
    const { result } = renderHook(() =>
      useDebouncedConfig('sync.default_interval', '5m', updateConfig)
    )
    expect(result.current[0]).toBe('5m')
  })

  it('debounces save after 1500ms when value changes', async () => {
    const updateConfig = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(({ val }) =>
      useDebouncedConfig('sync.default_interval', val, updateConfig)
    , { initialProps: { val: '5m' } })

    // Change local value
    await act(async () => {
      result.current[1]('10m')
    })

    // Not saved yet
    expect(updateConfig).not.toHaveBeenCalled()
    expect(result.current[2]).toBe(false)

    // Advance time to trigger debounce
    await act(async () => {
      vi.advanceTimersByTime(1500)
    })

    expect(updateConfig).toHaveBeenCalledWith('sync.default_interval', '10m')
  })

  it('does not save when value matches configValue', async () => {
    const updateConfig = vi.fn()
    const { result } = renderHook(() =>
      useDebouncedConfig('sync.default_interval', '5m', updateConfig)
    )

    await act(async () => {
      result.current[1]('5m')
    })

    await act(async () => {
      vi.advanceTimersByTime(2000)
    })

    expect(updateConfig).not.toHaveBeenCalled()
  })

  it('cancels pending save on cleanup', async () => {
    const updateConfig = vi.fn().mockResolvedValue(undefined)
    const { result, unmount } = renderHook(({ val }) =>
      useDebouncedConfig('sync.default_interval', val, updateConfig)
    , { initialProps: { val: '5m' } })

    await act(async () => {
      result.current[1]('10m')
    })

    unmount()

    await act(async () => {
      vi.advanceTimersByTime(2000)
    })

    expect(updateConfig).not.toHaveBeenCalled()
  })
})

describe('useDebouncedConfigMap', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('initializes all fields with empty strings', () => {
    const updateConfig = vi.fn()
    const fields = {
      interval: { configKey: 'sync.default_interval', configValue: undefined },
      timeout: { configKey: 'agent.timeout', configValue: undefined }
    }
    const { result } = renderHook(() =>
      useDebouncedConfigMap(fields, updateConfig)
    )
    expect(result.current.values).toEqual({ interval: '', timeout: '' })
    expect(result.current.savings).toEqual({ interval: false, timeout: false })
  })

  it('syncs values from configValue', () => {
    const updateConfig = vi.fn()
    const fields = {
      interval: { configKey: 'sync.default_interval', configValue: '5m' },
      timeout: { configKey: 'agent.timeout', configValue: '30s' }
    }
    const { result } = renderHook(() =>
      useDebouncedConfigMap(fields, updateConfig)
    )
    expect(result.current.values.interval).toBe('5m')
    expect(result.current.values.timeout).toBe('30s')
  })

  it('debounces save for changed fields', async () => {
    const updateConfig = vi.fn().mockResolvedValue(undefined)
    const fields = {
      interval: { configKey: 'sync.default_interval', configValue: '5m' }
    }
    const { result } = renderHook(() =>
      useDebouncedConfigMap(fields, updateConfig)
    )

    await act(async () => {
      result.current.setValue('interval', '10m')
    })

    expect(updateConfig).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(1500)
    })

    expect(updateConfig).toHaveBeenCalledWith('sync.default_interval', '10m')
  })
})
