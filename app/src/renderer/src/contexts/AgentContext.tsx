/**
 * AgentContext — manages AI agent state and resolve actions
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  useRef,
  type ReactNode
} from 'react'
import type { AgentInfo, AgentSessionInfo, ResolveData, AcceptData, AgentResetData } from '@/types/engine'
import { engineApi } from '@/lib/api'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

interface AgentState {
  agents: AgentInfo[]
  preferred: string
  sessions: AgentSessionInfo[]
  loading: boolean
  initialized: boolean
  error: string | null
}

type AgentAction =
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_AGENTS'; agents: AgentInfo[]; preferred: string }
  | { type: 'SET_AGENTS_SILENT'; agents: AgentInfo[]; preferred: string }
  | { type: 'SET_SESSIONS'; sessions: AgentSessionInfo[] }
  | { type: 'SET_SESSIONS_SILENT'; sessions: AgentSessionInfo[] }
  | { type: 'SET_ERROR'; error: string | null }

const initialState: AgentState = {
  agents: [],
  preferred: '',
  sessions: [],
  loading: false,
  initialized: false,
  error: null
}

function agentReducer(state: AgentState, action: AgentAction): AgentState {
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
  resolve: (
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ) => Promise<ResolveData | null>
  resolveAccept: (name: string) => Promise<AcceptData | null>
  resolveReject: (name: string) => Promise<boolean>
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

  const resolve = useCallback(
    async (
      name: string,
      opts?: { agent?: string; noConfirm?: boolean }
    ): Promise<ResolveData | null> => {
      dispatch({ type: 'SET_LOADING', loading: true })
      try {
        const res = await engineApi.resolve(name, opts)
        if (res.success) {
          dispatch({ type: 'SET_LOADING', loading: false })
          return res.data
        } else {
          dispatch({ type: 'SET_ERROR', error: res.error })
          return null
        }
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
        return null
      }
    },
    []
  )

  const resolveAccept = useCallback(async (name: string): Promise<AcceptData | null> => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.resolveAccept(name)
      if (res.success) {
        dispatch({ type: 'SET_LOADING', loading: false })
        return res.data
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
        return null
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      return null
    }
  }, [])

  const resolveReject = useCallback(async (name: string): Promise<boolean> => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.resolveReject(name)
      if (res.success) {
        dispatch({ type: 'SET_LOADING', loading: false })
        return true
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
        return false
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      return false
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
        resolve,
        resolveAccept,
        resolveReject,
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
