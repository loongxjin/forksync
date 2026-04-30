/**
 * AgentContext — manages AI agent state and resolve actions
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  useRef,
  useEffect,
  type ReactNode
} from 'react'
import type { AgentInfo, AgentSessionInfo, ResolveData, AcceptData, AgentResetData, AgentStreamEvent } from '@/types/engine'
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
  streamEvents: Record<string, AgentStreamEvent[]>
  streamLive: Set<string>
  streamResults: Record<string, ResolveData | null>
}

type AgentAction =
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_AGENTS'; agents: AgentInfo[]; preferred: string }
  | { type: 'SET_AGENTS_SILENT'; agents: AgentInfo[]; preferred: string }
  | { type: 'SET_SESSIONS'; sessions: AgentSessionInfo[] }
  | { type: 'SET_SESSIONS_SILENT'; sessions: AgentSessionInfo[] }
  | { type: 'SET_ERROR'; error: string | null }
  | { type: 'STREAM_START'; repoName: string }
  | { type: 'STREAM_EVENT'; repoName: string; event: AgentStreamEvent }
  | { type: 'STREAM_DONE'; repoName: string; result?: ResolveData | null }
  | { type: 'STREAM_LOAD'; repoName: string; events: AgentStreamEvent[]; isRunning: boolean }
  | { type: 'STREAM_CLEAR'; repoName: string }

const initialState: AgentState = {
  agents: [],
  preferred: '',
  sessions: [],
  loading: false,
  initialized: false,
  error: null,
  streamEvents: {},
  streamLive: new Set(),
  streamResults: {}
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
    case 'STREAM_START': {
      const nextLive = new Set(state.streamLive)
      nextLive.add(action.repoName)
      return {
        ...state,
        streamLive: nextLive,
        streamEvents: {
          ...state.streamEvents,
          [action.repoName]: []
        }
      }
    }
    case 'STREAM_EVENT': {
      const existing = state.streamEvents[action.repoName] ?? []
      return {
        ...state,
        streamEvents: {
          ...state.streamEvents,
          [action.repoName]: [...existing, action.event]
        }
      }
    }
    case 'STREAM_DONE': {
      const nextLive = new Set(state.streamLive)
      nextLive.delete(action.repoName)
      return {
        ...state,
        streamLive: nextLive,
        streamResults: action.result !== undefined
          ? { ...state.streamResults, [action.repoName]: action.result }
          : state.streamResults
      }
    }
    case 'STREAM_LOAD': {
      const nextLive = new Set(state.streamLive)
      if (action.isRunning) {
        nextLive.add(action.repoName)
      } else {
        nextLive.delete(action.repoName)
      }
      return {
        ...state,
        streamEvents: {
          ...state.streamEvents,
          [action.repoName]: action.events
        },
        streamLive: nextLive
      }
    }
    case 'STREAM_CLEAR': {
      const nextEvents = { ...state.streamEvents }
      delete nextEvents[action.repoName]
      const nextLive = new Set(state.streamLive)
      nextLive.delete(action.repoName)
      return { ...state, streamEvents: nextEvents, streamLive: nextLive }
    }
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
  resolveStream: (name: string, opts?: { agent?: string; noConfirm?: boolean }) => void
  loadAgentLog: (name: string) => Promise<void>
  clearStream: (name: string) => void
  streamResults: Record<string, ResolveData | null>
}

const AgentContext = createContext<AgentContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function AgentProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(agentReducer, initialState)
  const ipcSetupRef = useRef(false)

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

  const resolveStream = useCallback((name: string, opts?: { agent?: string; noConfirm?: boolean }) => {
    console.log('[AgentContext] resolveStream', name, opts)
    dispatch({ type: 'STREAM_START', repoName: name })
    engineApi.resolveStreamStart(name, opts)
  }, [])

  const loadAgentLog = useCallback(async (name: string): Promise<void> => {
    console.log('[AgentContext] loadAgentLog', name)
    try {
      const res = await engineApi.readAgentLog(name)
      console.log('[AgentContext] loadAgentLog result', name, res.events.length, 'events, isRunning:', res.isRunning)
      dispatch({ type: 'STREAM_LOAD', repoName: name, events: res.events, isRunning: res.isRunning })
    } catch (err) {
      console.error('[AgentContext] loadAgentLog failed', name, err)
      dispatch({ type: 'STREAM_LOAD', repoName: name, events: [], isRunning: false })
    }
  }, [])

  const clearStream = useCallback((name: string) => {
    dispatch({ type: 'STREAM_CLEAR', repoName: name })
  }, [])

  // Set up IPC listeners once
  useEffect(() => {
    if (ipcSetupRef.current) return
    ipcSetupRef.current = true

    const unsubEvent = engineApi.onResolveStreamEvent((repoName, event) => {
      console.log('[AgentContext] stream event', repoName, event.t)
      dispatch({ type: 'STREAM_EVENT', repoName, event })
    })

    const unsubDone = engineApi.onResolveStreamDone((repoName, apiRes) => {
      console.log('[AgentContext] stream done', repoName, apiRes.success)
      dispatch({ type: 'STREAM_DONE', repoName, result: apiRes.success ? apiRes.data : null })
    })

    const unsubError = engineApi.onResolveStreamError((repoName, error) => {
      console.error('[AgentContext] stream error', repoName, error)
      dispatch({ type: 'STREAM_EVENT', repoName, event: { t: 'error', d: error, ts: new Date().toISOString() } })
      dispatch({ type: 'STREAM_DONE', repoName })
    })

    return () => {
      unsubEvent()
      unsubDone()
      unsubError()
    }
  }, [])

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
        resetSession,
        resolveStream,
        loadAgentLog,
        clearStream,
        streamResults: state.streamResults
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
