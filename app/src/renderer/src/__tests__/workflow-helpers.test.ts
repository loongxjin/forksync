import { describe, it, expect } from 'vitest'
import { shouldShowResolveDetails, shouldShowStepDetail } from '@/lib/workflow-helpers'

describe('shouldShowResolveDetails', () => {
  it('returns false for null', () => {
    expect(shouldShowResolveDetails(null)).toBe(false)
  })

  it('returns false for undefined', () => {
    expect(shouldShowResolveDetails(undefined)).toBe(false)
  })

  it('returns false when resolveResult has no details', () => {
    const result = {
      success: true,
      data: { repoId: '1', conflicts: [], agentResult: null },
      error: ''
    } as any
    expect(shouldShowResolveDetails(result)).toBe(false)
  })

  it('returns true when agentResult has agentName', () => {
    const result = {
      agentResult: { agentName: 'claude', summary: '', resolvedFiles: [], diff: '' },
      conflicts: []
    } as any
    expect(shouldShowResolveDetails(result)).toBe(true)
  })

  it('returns true when agentResult has summary', () => {
    const result = {
      agentResult: { agentName: '', summary: 'AI resolved conflicts', resolvedFiles: [], diff: '' },
      conflicts: []
    } as any
    expect(shouldShowResolveDetails(result)).toBe(true)
  })

  it('returns true when there are conflicts', () => {
    const result = {
      agentResult: null,
      conflicts: [{ path: 'main.go' }]
    } as any
    expect(shouldShowResolveDetails(result)).toBe(true)
  })

  it('returns true when there is a commitError', () => {
    const result = {
      agentResult: null,
      conflicts: [],
      commitError: 'merge failed'
    } as any
    expect(shouldShowResolveDetails(result)).toBe(true)
  })
})

describe('shouldShowStepDetail', () => {
  it('returns true for agent_resolve success', () => {
    const step = { step: 'agent_resolve', status: 'success' }
    const steps = [step, { step: 'commit', status: 'waiting' }]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  it('returns true for agent_resolve failed', () => {
    const step = { step: 'agent_resolve', status: 'failed' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  it('returns false for agent_resolve running', () => {
    const step = { step: 'agent_resolve', status: 'running' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })

  it('returns true for accept_changes waiting when no agent_resolve finished', () => {
    const step = { step: 'accept_changes', status: 'waiting' }
    const steps = [
      { step: 'sync', status: 'success' },
      { step: 'resolve_strategy', status: 'success' },
      step
    ]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  it('returns false for accept_changes waiting when agent_resolve already finished', () => {
    const step = { step: 'accept_changes', status: 'waiting' }
    const steps = [
      { step: 'agent_resolve', status: 'success' },
      step
    ]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })

  it('returns false for unrelated steps', () => {
    const step = { step: 'sync', status: 'success' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })

  it('returns false for commit step waiting', () => {
    const step = { step: 'commit', status: 'waiting' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })
})
