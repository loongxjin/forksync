import { describe, it, expect } from 'vitest'
import { agentReducer, initialState } from '@/contexts/AgentContext'
import type { AgentAction } from '@/contexts/AgentContext'
import type { AgentStreamEvent } from '@shared/types/engine'

describe('agentReducer', () => {
  it('returns initial state for unknown action', () => {
    const state = agentReducer(initialState, { type: 'UNKNOWN' } as unknown as AgentAction)
    expect(state).toBe(initialState)
  })

  describe('SET_LOADING', () => {
    it('sets loading and clears error', () => {
      const state = { ...initialState, error: 'some error' }
      const next = agentReducer(state, { type: 'SET_LOADING', loading: true })
      expect(next.loading).toBe(true)
      expect(next.error).toBeNull()
    })
  })

  describe('SET_AGENTS', () => {
    it('sets agents, preferred, loading=false, initialized=true', () => {
      const agents = [{ name: 'claude', installed: true, preferred: true }]
      const next = agentReducer(
        { ...initialState, loading: true },
        { type: 'SET_AGENTS', agents, preferred: 'claude' }
      )
      expect(next.agents).toBe(agents)
      expect(next.preferred).toBe('claude')
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(true)
    })
  })

  describe('SET_AGENTS_SILENT', () => {
    it('sets agents and preferred without changing loading/initialized', () => {
      const next = agentReducer(initialState, {
        type: 'SET_AGENTS_SILENT',
        agents: [{ name: 'opencode', installed: true, preferred: false }],
        preferred: 'opencode'
      })
      expect(next.agents).toHaveLength(1)
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(false)
    })
  })

  describe('SET_SESSIONS', () => {
    it('sets sessions, loading=false, initialized=true', () => {
      const sessions = [{ id: '1', agentName: 'claude', repoName: 'repo1', status: 'active' }]
      const next = agentReducer(
        { ...initialState, loading: true },
        { type: 'SET_SESSIONS', sessions: sessions as any }
      )
      expect(next.sessions).toBe(sessions)
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(true)
    })
  })

  describe('SET_SESSIONS_SILENT', () => {
    it('sets sessions without changing loading/initialized', () => {
      const next = agentReducer(initialState, {
        type: 'SET_SESSIONS_SILENT',
        sessions: []
      })
      expect(next.sessions).toEqual([])
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(false)
    })
  })

  describe('SET_ERROR', () => {
    it('sets error and loading=false', () => {
      const next = agentReducer(
        { ...initialState, loading: true },
        { type: 'SET_ERROR', error: 'fail' }
      )
      expect(next.error).toBe('fail')
      expect(next.loading).toBe(false)
    })
  })

  describe('STREAM_START', () => {
    it('marks repo as live and resets events', () => {
      const prev = {
        ...initialState,
        streamEvents: { repo1: [{ t: 'old', d: '', ts: '' }] },
        streamLive: { repo1: true }
      }
      const next = agentReducer(prev, { type: 'STREAM_START', repoName: 'repo2' })
      expect(next.streamLive).toEqual({ repo1: true, repo2: true })
      expect(next.streamEvents['repo2']).toEqual([])
      // existing repo1 events untouched
      expect(next.streamEvents['repo1']).toHaveLength(1)
    })
  })

  describe('STREAM_EVENT', () => {
    it('appends event to existing events', () => {
      const event: AgentStreamEvent = { t: 'stdout', d: 'hello', ts: '2026-01-01' }
      const prev = {
        ...initialState,
        streamEvents: { repo1: [{ t: 'stdout', d: 'first', ts: '2026-01-01' }] }
      }
      const next = agentReducer(prev, { type: 'STREAM_EVENT', repoName: 'repo1', event })
      expect(next.streamEvents['repo1']).toHaveLength(2)
      expect(next.streamEvents['repo1'][1]).toBe(event)
    })

    it('creates events array if none exist', () => {
      const event: AgentStreamEvent = { t: 'stdout', d: 'hello', ts: '2026-01-01' }
      const next = agentReducer(initialState, { type: 'STREAM_EVENT', repoName: 'newrepo', event })
      expect(next.streamEvents['newrepo']).toHaveLength(1)
    })
  })

  describe('STREAM_DONE', () => {
    it('removes from live and sets result when provided', () => {
      const result = { success: true, data: { repoId: '1' }, error: '' } as any
      const prev = {
        ...initialState,
        streamLive: { repo1: true }
      }
      const next = agentReducer(prev, { type: 'STREAM_DONE', repoName: 'repo1', result })
      expect(next.streamLive).toEqual({})
      expect(next.streamResults['repo1']).toBe(result)
    })

    it('removes from live without touching results when result is undefined', () => {
      const prev = {
        ...initialState,
        streamLive: { repo1: true },
        streamResults: { repo1: null }
      }
      const next = agentReducer(prev, { type: 'STREAM_DONE', repoName: 'repo1' })
      expect(next.streamLive).toEqual({})
      // result is undefined → streamResults unchanged
      expect(next.streamResults).toEqual({ repo1: null })
    })
  })

  describe('STREAM_LOAD', () => {
    it('sets events and marks live when running', () => {
      const events: AgentStreamEvent[] = [{ t: 'stdout', d: 'data', ts: '' }]
      const next = agentReducer(initialState, {
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
        ...initialState,
        streamLive: { repo1: true }
      }
      const next = agentReducer(prev, {
        type: 'STREAM_LOAD',
        repoName: 'repo1',
        events: [],
        isRunning: false
      })
      expect(next.streamLive).toEqual({})
    })
  })

  describe('STREAM_CLEAR', () => {
    it('removes events and live status', () => {
      const prev = {
        ...initialState,
        streamEvents: { repo1: [{ t: 'stdout', d: '', ts: '' }], repo2: [] },
        streamLive: { repo1: true, repo2: true }
      }
      const next = agentReducer(prev, { type: 'STREAM_CLEAR', repoName: 'repo1' })
      expect(next.streamEvents['repo1']).toBeUndefined()
      expect(next.streamLive).toEqual({ repo2: true })
      // repo2 untouched
      expect(next.streamEvents['repo2']).toEqual([])
    })
  })
})
