/**
 * Scenario tests for the disk-log-as-single-source resolve stream.
 * Simulates the full lifecycle using only the reducer + extractResolveDataFromEvents,
 * no React rendering needed.
 *
 * Covers the 3 user requirements:
 * 1. Live output with no duplication (STREAM_LOAD always replaces)
 * 2. Close & reopen during active resolve (re-read continues from disk)
 * 3. Reopen after completion (full events + resolveResults from done frame)
 */

import { describe, it, expect } from 'vitest'
import type { AgentStreamEvent } from '@shared/types/engine'
import { streamReducer, initialStreamState, extractResolveDataFromEvents } from '@/hooks/useResolveStream'

// Event builders
function partialLog(): AgentStreamEvent[] {
  return [
    { t: 'start', agent: 'claude', files: ['README.md'], ts: '00:00:00' },
    { t: 'stdout', d: 'Analyzing...', ts: '00:00:01' },
    { t: 'tool', name: 'Bash', path: '', ts: '00:00:02' }
  ]
}

function grownLog(): AgentStreamEvent[] {
  return [
    ...partialLog(),
    { t: 'stdout', d: 'Resolving...', ts: '00:00:05' },
    { t: 'tool', name: 'Edit', path: '/README.md', ts: '00:00:06' }
  ]
}

function completedLog(): AgentStreamEvent[] {
  return [
    ...grownLog(),
    {
      t: 'done', success: true, summary: 'Resolved',
      session_id: 's1', resolvedFiles: ['README.md'],
      diff: 'diff content', agentName: 'claude', ts: '00:00:10'
    } as AgentStreamEvent
  ]
}

describe('Scenario 1: live output, no duplication', () => {
  it('STREAM_LOAD always replaces events — reading the same log twice gives same count', () => {
    let state = initialStreamState

    // First read: 3 events
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: partialLog(), isRunning: true })
    expect(state.streamEvents['repo']).toHaveLength(3)

    // Second read: same 3 events (tick + poll fired close together)
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: partialLog(), isRunning: true })
    expect(state.streamEvents['repo']).toHaveLength(3) // NOT 6

    // Third read: log grew to 5 events
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: grownLog(), isRunning: true })
    expect(state.streamEvents['repo']).toHaveLength(5) // NOT 8
  })
})

describe('Scenario 2: close & reopen during active resolve', () => {
  it('re-reading disk log after reopen continues from where it left off', () => {
    let state = initialStreamState

    // Initial load: agent running, 3 events
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: partialLog(), isRunning: true })
    expect(state.streamLive['repo']).toBe(true)

    // User closes terminal — state persists in memory
    // User reopens — readDiskLog fires again, log has grown
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: grownLog(), isRunning: true })

    // Full replacement — sees all 5 events, no gaps
    expect(state.streamEvents['repo']).toHaveLength(5)
    expect(state.streamLive['repo']).toBe(true)
  })
})

describe('Scenario 3: reopen after completion', () => {
  it('reading completed log restores full events + resolveResults from done frame', () => {
    let state = initialStreamState

    // Agent completed previously — load full log
    const events = completedLog()
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events, isRunning: false })

    // All 7 events preserved (5 + done = 6... wait, grownLog has 5, +done = 6)
    expect(state.streamEvents['repo']).toHaveLength(6)

    // resolveResults restored from done frame
    const data = state.resolveResults['repo']
    expect(data).toBeDefined()
    expect(data.conflicts).toEqual([{ path: 'README.md' }])
    expect(data.agentResult?.summary).toBe('Resolved')
    expect(data.agentResult?.diff).toBe('diff content')
    expect(data.agentResult?.agentName).toBe('claude')

    // Not live
    expect(state.streamLive['repo']).toBeUndefined()
  })

  it('polling picks up done frame and transitions to completed', () => {
    let state = initialStreamState

    // Poll: still running
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: partialLog(), isRunning: true })
    expect(state.streamLive['repo']).toBe(true)
    expect(state.resolveResults['repo']).toBeUndefined()

    // Poll: agent finished, done frame now in log
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: completedLog(), isRunning: false })

    // resolveResults now populated
    expect(state.resolveResults['repo']).toBeDefined()
    expect(state.resolveResults['repo'].agentResult?.resolvedFiles).toEqual(['README.md'])
    expect(state.streamLive['repo']).toBeUndefined()
  })
})

describe('Scenario 4: empty log race at start', () => {
  it('empty log with isRunning=false does NOT set resolveResults', () => {
    const state = streamReducer(initialStreamState, {
      type: 'STREAM_LOAD', repoName: 'repo', events: [], isRunning: false
    })
    expect(state.streamEvents['repo']).toHaveLength(0)
    expect(state.resolveResults['repo']).toBeUndefined()
  })
})

describe('Scenario 5: STREAM_CLEAR resets everything', () => {
  it('clearing after completion removes all traces', () => {
    let state = initialStreamState
    state = streamReducer(state, { type: 'STREAM_LOAD', repoName: 'repo', events: completedLog(), isRunning: false })

    // Clear (e.g. starting a new resolve)
    state = streamReducer(state, { type: 'STREAM_CLEAR', repoName: 'repo' })

    expect(state.streamEvents['repo']).toBeUndefined()
    expect(state.streamLive['repo']).toBeUndefined()
    expect(state.resolveResults['repo']).toBeUndefined()
    expect(state.streamResults['repo']).toBeUndefined()
  })
})
