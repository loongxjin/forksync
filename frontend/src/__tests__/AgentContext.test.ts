import { describe, it, expect } from 'vitest'
import { agentReducer, initialState } from '@/contexts/AgentContext'
import type { AgentAction } from '@/contexts/AgentContext'

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
})
