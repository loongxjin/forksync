import { describe, it, expect } from 'vitest'
import { shouldShowResolveDetails, shouldShowStepDetail, filterDiffByFile } from '@/lib/workflow-helpers'

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
  // accept_changes waiting → always show details
  it('returns true for accept_changes waiting (no agent)', () => {
    const step = { step: 'accept_changes', status: 'waiting' }
    const steps = [
      { step: 'sync', status: 'success' },
      { step: 'resolve_strategy', status: 'success' },
      step
    ]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  it('returns true for accept_changes waiting with agent_resolve finished', () => {
    const step = { step: 'accept_changes', status: 'waiting' }
    const steps = [
      { step: 'agent_resolve', status: 'success' },
      step
    ]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  // agent_resolve finished → show only while accept_changes hasn't reached waiting yet
  it('returns true for agent_resolve success without accept_changes', () => {
    const step = { step: 'agent_resolve', status: 'success' }
    const steps = [step, { step: 'commit', status: 'waiting' }]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  it('returns false for agent_resolve success when accept_changes is waiting', () => {
    const step = { step: 'agent_resolve', status: 'success' }
    const steps = [step, { step: 'accept_changes', status: 'waiting' }]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })

  it('returns true for agent_resolve failed without accept_changes waiting', () => {
    const step = { step: 'agent_resolve', status: 'failed' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(true)
  })

  it('returns false for agent_resolve running', () => {
    const step = { step: 'agent_resolve', status: 'running' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })

  it('returns false for unrelated steps', () => {
    const step = { step: 'sync', status: 'success' }
    const steps = [step]
    expect(shouldShowStepDetail(step, steps)).toBe(false)
  })
})

describe('filterDiffByFile', () => {
  const fullDiff = [
    'diff --git a/README.md b/README.md',
    '--- a/README.md',
    '+++ b/README.md',
    '@@ -1,3 +1,3 @@',
    '-# Old',
    '+# New',
    '',
    'diff --git a/src/main.go b/src/main.go',
    '--- a/src/main.go',
    '+++ b/src/main.go',
    '@@ -1 +1 @@',
    '-old',
    '+new',
  ].join('\n')

  it('returns only the matching file section', () => {
    const result = filterDiffByFile(fullDiff, 'src/main.go')
    expect(result).toContain('diff --git a/src/main.go')
    expect(result).toContain('+new')
    expect(result).not.toContain('README.md')
  })

  it('returns only the other file section', () => {
    const result = filterDiffByFile(fullDiff, 'README.md')
    expect(result).toContain('# New')
    expect(result).not.toContain('src/main.go')
  })

  it('returns full diff when file not found', () => {
    const result = filterDiffByFile(fullDiff, 'nonexistent.txt')
    expect(result).toBe(fullDiff)
  })

  it('returns empty diff as-is', () => {
    expect(filterDiffByFile('', 'any.txt')).toBe('')
  })
})
