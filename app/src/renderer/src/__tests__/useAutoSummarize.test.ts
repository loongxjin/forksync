import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useAutoSummarize } from '@/hooks/useAutoSummarize'

// Mock dependencies
vi.mock('@/lib/api', () => ({
  engineApi: {
    summarize: vi.fn().mockResolvedValue({ success: true })
  }
}))

vi.mock('@/contexts/SettingsContext', () => ({
  useSettings: vi.fn()
}))

import { engineApi } from '@/lib/api'
import { useSettings } from '@/contexts/SettingsContext'

describe('useAutoSummarize', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls engineApi.summarize when AutoSummary is enabled', () => {
    vi.mocked(useSettings).mockReturnValue({
      engineConfig: { Sync: { AutoSummary: true } }
    } as any)

    const { result } = renderHook(() => useAutoSummarize())
    result.current.triggerSummarize('my-repo')

    expect(engineApi.summarize).toHaveBeenCalledWith('my-repo')
  })

  it('does not call summarize when AutoSummary is disabled', () => {
    vi.mocked(useSettings).mockReturnValue({
      engineConfig: { Sync: { AutoSummary: false } }
    } as any)

    const { result } = renderHook(() => useAutoSummarize())
    result.current.triggerSummarize('my-repo')

    expect(engineApi.summarize).not.toHaveBeenCalled()
  })

  it('does not call summarize when engineConfig is undefined', () => {
    vi.mocked(useSettings).mockReturnValue({
      engineConfig: undefined
    } as any)

    const { result } = renderHook(() => useAutoSummarize())
    result.current.triggerSummarize('my-repo')

    expect(engineApi.summarize).not.toHaveBeenCalled()
  })

  it('swallows summarize errors silently', async () => {
    vi.mocked(useSettings).mockReturnValue({
      engineConfig: { Sync: { AutoSummary: true } }
    } as any)
    vi.mocked(engineApi.summarize).mockRejectedValueOnce(new Error('network'))

    const { result } = renderHook(() => useAutoSummarize())
    // Should not throw
    result.current.triggerSummarize('my-repo')

    // Let the microtask settle
    await vi.waitFor(() => {
      expect(engineApi.summarize).toHaveBeenCalledWith('my-repo')
    })
  })
})
