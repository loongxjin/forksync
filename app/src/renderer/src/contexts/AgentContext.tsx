/**
 * AgentContext — manages AI agent state (agents list, sessions, cleanup)
 *
 * Resolve/stream logic has been extracted to useResolveStream hook.
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  type ReactNode
} from 'react'
import type { AgentInfo, AgentSessionInfo, AgentResetData } from '@shared/types/engine'
import { engineApi } from '@/lib/api'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

export interface AgentState {
  agents: AgentInfo[]
  preferred: string
  sessions: AgentSessionInfo[]
  loading: boolean
  initialized: boolean
  error: string | null
}

export type AgentAction =
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_AGENTS'; agents: AgentInfo[]; preferred: string }
  | { type: 'SET_AGENTS_SILENT'; agents: AgentInfo[]; preferred: string }
  | { type: 'SET_SESSIONS'; sessions: AgentSessionInfo[] }
  | { type: 'SET_SESSIONS_SILENT'; sessions: AgentSessionInfo[] }
  | { type: 'SET_ERROR'; error: string | null }

export const initialState: AgentState = {
  agents: [],
  preferred: '',
  sessions: [],
  loading: false,
  initialized: false,
  error: null
}

export function agentReducer(state: AgentState, action: AgentAction): AgentState {
  switch (action.type) {
    case 'SET_LOADING':
      return { ...state, loading: action.loading, error: null }
    case 'SET_AGENTS':
      return {
        ...state,
        agents: action.agents,
        preferred: action.preferred,
        loading: false,
        initialized: true
      }
    case 'SET_AGENTS_SILENT':
      return {
        ...state,
        agents: action.agents,
        preferred: action.preferred
      }
    case 'SET_SESSIONS':
      return { ...state, sessions: action.sessions, loading: false, initialized: true }
    case 'SET_SESSIONS_SILENT':
      return { ...state, sessions: action.sessions }
    case 'SET_ERROR':
      return { ...state, error: action.error, loading: false }
    default:
      return state
  }
}

// ---------------------------------------------------------------------------
// Context interface
// ---------------------------------------------------------------------------

interface AgentContextValue extends AgentState {
  refreshAgents: () => Promise<void>
  refreshSessions: () => Promise<void>
  cleanup: () => Promise<number>
  resetSession: (name: string) => Promise<AgentResetData | null>
}

const AgentContext = createContext<AgentContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function AgentProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(agentReducer, initialState)

  const refreshAgents = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.agentList()
      if (res.success) {
        dispatch({
          type: 'SET_AGENTS',
          agents: res.data.agents ?? [],
          preferred: res.data.preferred ?? ''
        })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [])

  const refreshSessions = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.agentSessions()
      if (res.success) {
        dispatch({ type: 'SET_SESSIONS', sessions: res.data.sessions ?? [] })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [])

  const cleanup = useCallback(async (): Promise<number> => {
    try {
      const res = await engineApi.agentCleanup()
      if (res.success) {
        await refreshSessions()
        return res.data.removed
      }
      return 0
    } catch {
      return 0
    }
  }, [refreshSessions])

  const resetSession = useCallback(async (name: string): Promise<AgentResetData | null> => {
    try {
      const res = await engineApi.agentReset(name)
      if (res.success) {
        await refreshSessions()
        return res.data
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
        return null
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      return null
    }
  }, [refreshSessions])

  return (
    <AgentContext.Provider
      value={{
        ...state,
        refreshAgents,
        refreshSessions,
        cleanup,
        resetSession
      }}
    >
      {children}
    </AgentContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useAgents(): AgentContextValue {
  const ctx = useContext(AgentContext)
  if (!ctx) {
    throw new Error('useAgents must be used within an AgentProvider')
  }
  return ctx
}
