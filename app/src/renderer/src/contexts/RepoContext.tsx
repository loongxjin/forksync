/**
 * RepoContext — manages repository state and engine actions
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  useRef,
  type ReactNode
} from 'react'
import type { Repo, ScannedRepo, SyncResult, BranchMapping } from '@/types/engine'
import { engineApi } from '@/lib/api'
import { isConflictStatus } from '@/lib/utils'
import type { ToastState } from '@/components/ui/toast'
import i18n from '@/i18n'
import { useSettings } from '@/contexts/SettingsContext'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

interface RepoState {
  repos: Repo[]
  scannedRepos: ScannedRepo[]
  syncResults: SyncResult[]
  loading: boolean
  initialized: boolean
  error: string | null
  toast: ToastState
}

type RepoAction =
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_INITIALIZED' }
  | { type: 'SET_REPOS'; repos: Repo[] }
  | { type: 'SET_REPOS_SILENT'; repos: Repo[] }
  | { type: 'SET_SCANNED'; repos: ScannedRepo[] }
  | { type: 'SET_SYNC_RESULTS'; results: SyncResult[] }
  | { type: 'UPDATE_REPO'; repo: Repo }
  | { type: 'SET_REPO_STATUS'; repoId: string; status: Repo['status'] }
  | { type: 'REMOVE_REPO'; repoId: string }
  | { type: 'SET_ERROR'; error: string | null }
  | { type: 'SHOW_TOAST'; message: string; toastType: ToastState['type'] }
  | { type: 'HIDE_TOAST' }

const initialState: RepoState = {
  repos: [],
  scannedRepos: [],
  syncResults: [],
  loading: false,
  initialized: false,
  error: null,
  toast: { message: '', visible: false, type: 'info' }
}

function repoReducer(state: RepoState, action: RepoAction): RepoState {
  switch (action.type) {
    case 'SET_LOADING':
      return { ...state, loading: action.loading, error: null }
    case 'SET_INITIALIZED':
      return { ...state, initialized: true }
    case 'SET_REPOS':
      return { ...state, repos: action.repos, loading: false, initialized: true }
    case 'SET_REPOS_SILENT':
      return { ...state, repos: action.repos }
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
    case 'SET_REPO_STATUS':
      return {
        ...state,
        repos: state.repos.map((r) =>
          r.id === action.repoId ? { ...r, status: action.status } : r
        )
      }
    case 'SET_ERROR':
      return { ...state, error: action.error, loading: false }
    case 'SHOW_TOAST':
      return {
        ...state,
        toast: { message: action.message, visible: true, type: action.toastType }
      }
    case 'HIDE_TOAST':
      return { ...state, toast: { ...state.toast, visible: false } }
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
  addRepo: (path: string, upstream?: string, branchMapping?: BranchMapping) => Promise<void>
  removeRepo: (name: string) => Promise<void>
  updateRepoStatus: (repoId: string, status: Repo['status']) => void
  showToast: (message: string, type?: ToastState['type']) => void
  hideToast: () => void
  startupSyncDone: boolean
  markStartupSyncDone: () => void
}

const RepoContext = createContext<RepoContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

const TOAST_DURATION = 2000 // 2 seconds

export function RepoProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(repoReducer, initialState)
  const toastTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const { engineConfig } = useSettings()

  // Guard against concurrent refresh calls
  const refreshingRef = useRef(false)

  const refresh = useCallback(async () => {
    if (refreshingRef.current) return
    refreshingRef.current = true
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.status()
      if (res.success) {
        dispatch({ type: 'SET_REPOS', repos: res.data.repos ?? [] })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    } finally {
      refreshingRef.current = false
    }
  }, [])

  const syncAll = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.syncAll()
      if (res.success) {
        const results = res.data.results ?? []
        dispatch({ type: 'SET_SYNC_RESULTS', results })

        // Toast feedback for batch sync
        const conflicts = results.filter(
          (r) => isConflictStatus(r.status)
        )
        const errors = results.filter((r) => r.status === 'error')
        if (conflicts.length > 0) {
          showToast(i18n.t('toast.syncConflicts', { count: conflicts.length }), 'warning')
        } else if (errors.length > 0) {
          showToast(i18n.t('toast.syncFailed', { count: errors.length }), 'error')
        } else {
          const totalCommits = results.reduce((s, r) => s + (r.commitsPulled ?? 0), 0)
          if (totalCommits > 0) {
            showToast(
              i18n.t('toast.syncSuccess', { count: results.length, pulled: totalCommits }),
              'success'
            )
          }
        }
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
      await refresh()

      // Fire-and-forget AI summarization for synced repos with commits
      if (engineConfig?.Sync?.AutoSummary && res.success) {
        const results = res.data.results ?? []
        for (const r of results) {
          if (r.status === 'synced' && (r.commitsPulled ?? 0) > 0) {
            engineApi.summarize(r.repoName).catch(() => {
              // ignore background summary errors
            })
          }
        }
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [refresh, engineConfig])

  // Toast functions must be defined before syncRepo to avoid TDZ error
  const hideToast = useCallback(() => {
    if (toastTimeoutRef.current) {
      clearTimeout(toastTimeoutRef.current)
      toastTimeoutRef.current = null
    }
    dispatch({ type: 'HIDE_TOAST' })
  }, [])

  const showToast = useCallback((message: string, toastType: ToastState['type'] = 'info') => {
    // Clear any existing timeout
    if (toastTimeoutRef.current) {
      clearTimeout(toastTimeoutRef.current)
    }
    dispatch({ type: 'SHOW_TOAST', message, toastType })
    // Auto-hide after duration
    toastTimeoutRef.current = setTimeout(() => {
      dispatch({ type: 'HIDE_TOAST' })
    }, TOAST_DURATION)
  }, [])

  // Track syncing repos to prevent duplicate sync requests
  const syncingReposRef = useRef<Set<string>>(new Set())

  // Track whether startup sync has been done (persists across page navigation)
  const startupSyncDoneRef = useRef(false)

  const syncRepo = useCallback(
    async (name: string) => {
      // Prevent duplicate sync for the same repo
      if (syncingReposRef.current.has(name)) {
        return
      }

      // Don't allow sync if repo is in conflict/resolving/resolved state
      const repo = state.repos.find((r) => r.name === name)
      if (repo && isConflictStatus(repo.status)) {
        showToast(i18n.t('toast.conflictsWarning', { name }), 'warning')
        return
      }

      syncingReposRef.current.add(name)

      dispatch({ type: 'SET_LOADING', loading: true })
      try {
        const res = await engineApi.syncRepo(name)
        if (res.success) {
          dispatch({ type: 'SET_SYNC_RESULTS', results: res.data.results })
          // Check if repo is up to date and show toast
          const upToDateResult = res.data.results?.find(
            (r) => r.status === 'up_to_date' && r.repoName === name
          )
          if (upToDateResult) {
            showToast(i18n.t('toast.upToDate', { name }), 'info')
          }
        } else {
          dispatch({ type: 'SET_ERROR', error: res.error })
        }
        await refresh()

        // Fire-and-forget AI summarization if auto_summary is enabled
        if (engineConfig?.Sync?.AutoSummary && res.success) {
          const r = res.data.results?.find((x) => x.repoName === name)
          if (r && r.status === 'synced' && (r.commitsPulled ?? 0) > 0) {
            engineApi.summarize(name).catch(() => {
              // ignore background summary errors
            })
          }
        }
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      } finally {
        syncingReposRef.current.delete(name)
      }
    },
    [refresh, showToast, engineConfig]
  )

  const scan = useCallback(async (dir: string) => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.scan(dir)
      if (res.success) {
        dispatch({ type: 'SET_SCANNED', repos: res.data.repos ?? [] })
      } else {
        dispatch({ type: 'SET_ERROR', error: res.error })
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
    }
  }, [])

  const addRepo = useCallback(
    async (path: string, upstream?: string, branchMapping?: BranchMapping) => {
      dispatch({ type: 'SET_LOADING', loading: true })
      try {
        const res = await engineApi.add(path, upstream, branchMapping)
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

  const markStartupSyncDone = useCallback(() => {
    startupSyncDoneRef.current = true
  }, [])

  const updateRepoStatus = useCallback((repoId: string, status: Repo['status']) => {
    dispatch({ type: 'SET_REPO_STATUS', repoId, status })
  }, [])

  return (
    <RepoContext.Provider
      value={{ ...state, refresh, syncAll, syncRepo, scan, addRepo, removeRepo, updateRepoStatus, showToast, hideToast, startupSyncDone: startupSyncDoneRef.current, markStartupSyncDone }}
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
