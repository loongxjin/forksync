/**
 * RepoContext — manages repository state and engine actions
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  type ReactNode
} from 'react'
import type { Repo, ScannedRepo, SyncResult } from '@/types/engine'
import { engineApi } from '@/lib/api'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

interface RepoState {
  repos: Repo[]
  scannedRepos: ScannedRepo[]
  syncResults: SyncResult[]
  loading: boolean
  error: string | null
}

type RepoAction =
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_REPOS'; repos: Repo[] }
  | { type: 'SET_SCANNED'; repos: ScannedRepo[] }
  | { type: 'SET_SYNC_RESULTS'; results: SyncResult[] }
  | { type: 'UPDATE_REPO'; repo: Repo }
  | { type: 'REMOVE_REPO'; repoId: string }
  | { type: 'SET_ERROR'; error: string | null }

const initialState: RepoState = {
  repos: [],
  scannedRepos: [],
  syncResults: [],
  loading: false,
  error: null
}

function repoReducer(state: RepoState, action: RepoAction): RepoState {
  switch (action.type) {
    case 'SET_LOADING':
      return { ...state, loading: action.loading, error: null }
    case 'SET_REPOS':
      return { ...state, repos: action.repos, loading: false }
    case 'SET_SCANNED':
      return { ...state, scannedRepos: action.repos, loading: false }
    case 'SET_SYNC_RESULTS':
      return { ...state, syncResults: action.results, loading: false }
    case 'UPDATE_REPO':
      return {
        ...state,
        repos: state.repos.map((r) => (r.id === action.repo.id ? action.repo : r))
      }
    case 'REMOVE_REPO':
      return {
        ...state,
        repos: state.repos.filter((r) => r.id !== action.repoId)
      }
    case 'SET_ERROR':
      return { ...state, error: action.error, loading: false }
    default:
      return state
  }
}

// ---------------------------------------------------------------------------
// Context interface
// ---------------------------------------------------------------------------

interface RepoContextValue extends RepoState {
  refresh: () => Promise<void>
  syncAll: () => Promise<void>
  syncRepo: (name: string) => Promise<void>
  scan: (dir: string) => Promise<void>
  addRepo: (path: string, upstream?: string) => Promise<void>
  removeRepo: (name: string) => Promise<void>
}

const RepoContext = createContext<RepoContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function RepoProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(repoReducer, initialState)

  const refresh = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.status()
      if (res.success) {
        dispatch({ type: 'SET_REPOS', repos: res.data.repos })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [])

  const syncAll = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.syncAll()
      if (res.success) {
        dispatch({ type: 'SET_SYNC_RESULTS', results: res.data.results })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
      // Refresh repo statuses after sync
      await refresh()
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [refresh])

  const syncRepo = useCallback(
    async (name: string) => {
      dispatch({ type: 'SET_LOADING', loading: true })
      try {
        const res = await engineApi.syncRepo(name)
        if (res.success) {
          dispatch({ type: 'SET_SYNC_RESULTS', results: res.data.results })
        } else {
          dispatch({ type: 'SET_ERROR', error: res.error })
        }
        await refresh()
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      }
    },
    [refresh]
  )

  const scan = useCallback(async (dir: string) => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.scan(dir)
      if (res.success) {
        dispatch({ type: 'SET_SCANNED', repos: res.data.repos })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [])

  const addRepo = useCallback(
    async (path: string, upstream?: string) => {
      dispatch({ type: 'SET_LOADING', loading: true })
      try {
        const res = await engineApi.add(path, upstream)
        if (res.success) {
          // Refresh full list after add
          await refresh()
        } else {
          dispatch({ type: 'SET_ERROR', error: res.error })
        }
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      }
    },
    [refresh]
  )

  const removeRepo = useCallback(
    async (name: string) => {
      dispatch({ type: 'SET_LOADING', loading: true })
      try {
        const res = await engineApi.remove(name)
        if (res.success) {
          await refresh()
        } else {
          dispatch({ type: 'SET_ERROR', error: res.error })
        }
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      }
    },
    [refresh]
  )

  return (
    <RepoContext.Provider
      value={{ ...state, refresh, syncAll, syncRepo, scan, addRepo, removeRepo }}
    >
      {children}
    </RepoContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useRepos(): RepoContextValue {
  const ctx = useContext(RepoContext)
  if (!ctx) {
    throw new Error('useRepos must be used within a RepoProvider')
  }
  return ctx
}
