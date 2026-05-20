import { describe, it, expect } from 'vitest'
import type { AgentStreamEvent, ResolveData } from '@shared/types/engine'
import { streamReducer, initialStreamState } from '@/hooks/useResolveStream'

describe('streamReducer', () => {
  describe('STREAM_START', () => {
    it('marks repo as live and resets events', () => {
      const prev = {
        ...initialStreamState,
        streamEvents: { repo1: [{ t: 'stdout' as const, d: 'old', ts: '' }] },
        streamLive: { repo1: true }
      }
      const next = streamReducer(prev, { type: 'STREAM_START', repoName: 'repo2' })
      expect(next.streamLive).toEqual({ repo1: true, repo2: true })
      expect(next.streamEvents['repo2']).toEqual([])
      expect(next.streamEvents['repo1']).toHaveLength(1)
    })
  })

  describe('STREAM_EVENT', () => {
    it('appends event to existing events', () => {
      const event: AgentStreamEvent = { t: 'stdout', d: 'hello', ts: '2026-01-01' }
      const prev = {
        ...initialStreamState,
        streamEvents: { repo1: [{ t: 'stdout' as const, d: 'first', ts: '2026-01-01' }] }
      }
      const next = streamReducer(prev, { type: 'STREAM_EVENT', repoName: 'repo1', event })
      expect(next.streamEvents['repo1']).toHaveLength(2)
      expect(next.streamEvents['repo1'][1]).toBe(event)
    })

    it('creates events array if none exist', () => {
      const event: AgentStreamEvent = { t: 'stdout', d: 'hello', ts: '2026-01-01' }
      const next = streamReducer(initialStreamState, { type: 'STREAM_EVENT', repoName: 'newrepo', event })
      expect(next.streamEvents['newrepo']).toHaveLength(1)
    })
  })

  describe('STREAM_DONE', () => {
    it('removes from live and sets result + resolveResults when result provided', () => {
      const result: ResolveData = { repoId: '1', conflicts: [] }
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

  describe('STREAM_LOAD', () => {
    it('sets events and marks live when running', () => {
      const events: AgentStreamEvent[] = [{ t: 'stdout', d: 'data', ts: '' }]
      const next = streamReducer(initialStreamState, {
        type: 'STREAM_LOAD',
        repoName: 'repo1',
        events,
        isRunning: true
      })
      expect(next.streamEvents['repo1']).toBe(events)
      expect(next.streamLive['repo1']).toBe(true)
    })

    it('sets events and removes from live when not running', () => {
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

  describe('MERGE_SYNC_RESULTS', () => {
    it('merges resolved sync results into resolveResults', () => {
      const data: ResolveData = { repoId: '1', conflicts: [] }
      const next = streamReducer(initialStreamState, {
        type: 'MERGE_SYNC_RESULTS',
        resolved: [{ repoName: 'repo1', data }]
      })
      expect(next.resolveResults['repo1']).toBe(data)
    })

    it('returns same state when resolved is empty', () => {
      const next = streamReducer(initialStreamState, {
        type: 'MERGE_SYNC_RESULTS',
        resolved: []
      })
      expect(next).toBe(initialStreamState)
    })
  })
})
