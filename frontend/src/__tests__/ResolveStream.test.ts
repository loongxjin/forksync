import { describe, it, expect } from 'vitest'
import type { AgentStreamEvent } from '@shared/types/engine'
import { streamReducer, initialStreamState, extractResolveDataFromEvents } from '@/hooks/useResolveStream'

describe('streamReducer', () => {
  describe('STREAM_LOAD', () => {
    it('replaces events and marks live when running', () => {
      const events: AgentStreamEvent[] = [{ t: 'stdout', d: 'data', ts: '' }]
      const prev = {
        ...initialStreamState,
        streamEvents: { repo1: [{ t: 'stdout' as const, d: 'old', ts: '' }] }
      }
      const next = streamReducer(prev, {
        type: 'STREAM_LOAD',
        repoName: 'repo1',
        events,
        isRunning: true
      })
      expect(next.streamEvents['repo1']).toBe(events)
      expect(next.streamLive['repo1']).toBe(true)
    })

    it('removes from live when not running', () => {
      const prev = {
        ...initialStreamState,
        streamLive: { repo1: true }
      }
      const next = streamReducer(prev, {
        type: 'STREAM_LOAD',
        repoName: 'repo1',
        events: [],
        isRunning: false
      })
      expect(next.streamLive).toEqual({})
    })

    it('restores resolveResults from done frame in events', () => {
      const events: AgentStreamEvent[] = [
        { t: 'start', agent: 'claude', files: ['README.md'], ts: '' },
        { t: 'stdout', d: 'working', ts: '' },
        {
          t: 'done',
          success: true,
          summary: 'resolved',
          session_id: 'sess-1',
          resolvedFiles: ['README.md', 'config.yaml'],
          diff: 'diff content',
          agentName: 'claude',
          ts: ''
        } as AgentStreamEvent
      ]
      const next = streamReducer(initialStreamState, {
        type: 'STREAM_LOAD',
        repoName: 'repo1',
        events,
        isRunning: false
      })
      expect(next.resolveResults['repo1']).toBeDefined()
      expect(next.resolveResults['repo1'].conflicts).toEqual([
        { path: 'README.md' },
        { path: 'config.yaml' }
      ])
      expect(next.resolveResults['repo1'].agentResult?.summary).toBe('resolved')
      expect(next.resolveResults['repo1'].agentResult?.diff).toBe('diff content')
    })

    it('does not set resolveResults when no done frame', () => {
      const events: AgentStreamEvent[] = [{ t: 'stdout', d: 'still running', ts: '' }]
      const next = streamReducer(initialStreamState, {
        type: 'STREAM_LOAD',
        repoName: 'repo1',
        events,
        isRunning: true
      })
      expect(next.resolveResults['repo1']).toBeUndefined()
    })
  })

  describe('STREAM_DONE', () => {
    it('removes from live and sets result + resolveResults when result provided', () => {
      const result = { repoId: '1', conflicts: [], agentResult: { success: true, summary: '', sessionId: '', agentName: '', resolvedFiles: [], diff: '' } }
      const prev = {
        ...initialStreamState,
        streamLive: { repo1: true }
      }
      const next = streamReducer(prev, { type: 'STREAM_DONE', repoName: 'repo1', result })
      expect(next.streamLive).toEqual({})
      expect(next.streamResults['repo1']).toBe(result)
      expect(next.resolveResults['repo1']).toBe(result)
    })

    it('removes from live without touching results when result is undefined', () => {
      const prev = {
        ...initialStreamState,
        streamLive: { repo1: true },
        streamResults: { repo1: null }
      }
      const next = streamReducer(prev, { type: 'STREAM_DONE', repoName: 'repo1' })
      expect(next.streamLive).toEqual({})
      expect(next.streamResults).toEqual({ repo1: null })
    })

    it('does not add to resolveResults when result is null', () => {
      const prev = {
        ...initialStreamState,
        streamLive: { repo1: true },
        resolveResults: {}
      }
      const next = streamReducer(prev, { type: 'STREAM_DONE', repoName: 'repo1', result: null })
      expect(next.streamLive).toEqual({})
      expect(next.resolveResults).toEqual({})
    })
  })

  describe('STREAM_CLEAR', () => {
    it('removes events, live status, and resolveResult for the repo', () => {
      const prev = {
        ...initialStreamState,
        streamEvents: { repo1: [{ t: 'stdout' as const, d: '', ts: '' }], repo2: [] },
        streamLive: { repo1: true, repo2: true },
        resolveResults: { repo1: { repoId: '1', conflicts: [] } }
      }
      const next = streamReducer(prev, { type: 'STREAM_CLEAR', repoName: 'repo1' })
      expect(next.streamEvents['repo1']).toBeUndefined()
      expect(next.streamLive).toEqual({ repo2: true })
      expect(next.streamEvents['repo2']).toEqual([])
      expect(next.resolveResults['repo1']).toBeUndefined()
    })
  })
})

describe('extractResolveDataFromEvents', () => {
  it('returns null for empty events', () => {
    expect(extractResolveDataFromEvents([])).toBeNull()
  })

  it('returns null when no done frame', () => {
    const events: AgentStreamEvent[] = [{ t: 'stdout', d: 'working', ts: '' }]
    expect(extractResolveDataFromEvents(events)).toBeNull()
  })

  it('extracts from the LAST done frame (not earlier ones)', () => {
    const events: AgentStreamEvent[] = [
      { t: 'done', success: true, summary: 'first', resolvedFiles: ['a.txt'], ts: '' } as AgentStreamEvent,
      { t: 'stdout', d: 'more work', ts: '' },
      { t: 'done', success: true, summary: 'second', resolvedFiles: ['b.txt'], ts: '' } as AgentStreamEvent
    ]
    const data = extractResolveDataFromEvents(events)
    expect(data).not.toBeNull()
    expect(data!.agentResult?.summary).toBe('second')
    expect(data!.conflicts).toEqual([{ path: 'b.txt' }])
  })
})
