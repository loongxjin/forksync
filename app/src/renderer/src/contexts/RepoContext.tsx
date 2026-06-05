/**
 * RepoContext — manages repository state and engine actions
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  useEffect,
  useRef,
  type ReactNode
} from 'react'
import type { Repo, ScannedRepo, SyncResult, BranchMapping } from '@shared/types/engine'
import { engineApi } from '@/lib/api'
import { isConflictStatus } from '@/lib/utils'
import i18n from '@/i18n'
import { useSettings } from '@/contexts/SettingsContext'
import { useAutoSummarize } from '@/hooks/useAutoSummarize'
import { useToastContext } from '@/contexts/ToastContext'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

export interface RepoState {
  repos: Repo[]
  scannedRepos: ScannedRepo[]
  syncResults: SyncResult[]
  loading: boolean
  initialized: boolean
  error: string | null
}

export type RepoAction =
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

export const initialState: RepoState = {
  repos: [],
  scannedRepos: [],
  syncResults: [],
  loading: false,
  initialized: false,
  error: null
}

export function repoReducer(state: RepoState, action: RepoAction): RepoState {
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
  updateRepo: (repo: Repo) => void
  startupSyncDone: boolean
  markStartupSyncDone: () => void
}

const RepoContext = createContext<RepoContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function RepoProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(repoReducer, initialState)
  const { engineConfig } = useSettings()
  const { triggerSummarize } = useAutoSummarize()
  const { showToast } = useToastContext()

  // Guard against concurrent refresh calls
  const refreshingRef = useRef(false)

  // Guard against concurrent syncAll calls
  const syncingAllRef = useRef(false)

  // Poll repos during sync so the UI shows workflow progress
  const syncPollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const syncPollCancelledRef = useRef(false)

  // Reference-counted polling: multiple operations (syncAll, syncRepo) may run
  // concurrently. Only stop the interval when the last consumer calls stopSyncPoll.
  const syncPollCountRef = useRef(0)

  // Start polling repo status while sync is running (agent resolve can take minutes)
  const startSyncPoll = useCallback(() => {
    syncPollCountRef.current++
    if (syncPollRef.current) return // already polling
    syncPollCancelledRef.current = false
    syncPollRef.current = setInterval(async () => {
      try {
        const res = await engineApi.status()
        if (res.success && !syncPollCancelledRef.current) {
          dispatch({ type: 'SET_REPOS_SILENT', repos: res.data.repos ?? [] })
        }
      } catch {
        // silent — polling is best-effort
      }
    }, 3000)
  }, [])

  const stopSyncPoll = useCallback(() => {
    syncPollCountRef.current = Math.max(0, syncPollCountRef.current - 1)
    if (syncPollCountRef.current > 0) return // other consumers still active
    syncPollCancelledRef.current = true
    if (syncPollRef.current) {
      clearInterval(syncPollRef.current)
      syncPollRef.current = null
    }
  }, [])

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
    // Prevent concurrent syncAll
    if (syncingAllRef.current) return
    syncingAllRef.current = true

    // Optimistically set all non-conflict repos to syncing
    for (const repo of state.repos) {
      if (!isConflictStatus(repo.status)) {
        dispatch({ type: 'SET_REPO_STATUS', repoId: repo.id, status: 'syncing' })
      }
    }

    dispatch({ type: 'SET_LOADING', loading: true })
    startSyncPoll() // poll repo status during sync to show workflow progress
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
      if (res.success) {
        const results = res.data.results ?? []
        for (const r of results) {
          if (r.status === 'up_to_date' && (r.commitsPulled ?? 0) > 0) {
            triggerSummarize(r.repoName)
          }
        }
      }
    } catch (err) {
      dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      await refresh()
    } finally {
      stopSyncPoll()
      syncingAllRef.current = false
    }
  }, [state, refresh, engineConfig, showToast, startSyncPoll, stopSyncPoll])

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

      // Optimistically set syncing status
      if (repo) {
        dispatch({ type: 'SET_REPO_STATUS', repoId: repo.id, status: 'syncing' })
      }

      dispatch({ type: 'SET_LOADING', loading: true })
      startSyncPoll() // poll repo status during sync to show workflow progress
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
        if (res.success) {
          const r = res.data.results?.find((x) => x.repoName === name)
          if (r && r.status === 'up_to_date' && (r.commitsPulled ?? 0) > 0) {
            triggerSummarize(name)
          }
        }
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
        await refresh()
      } finally {
        stopSyncPoll()
        syncingReposRef.current.delete(name)
      }
    },
    [state, refresh, showToast, engineConfig, startSyncPoll, stopSyncPoll]
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
      const repo = state.repos.find((r) => r.name === name)
      try {
        const res = await engineApi.remove(name)
        if (res.success && repo) {
          dispatch({ type: 'REMOVE_REPO', repoId: repo.id })
        } else {
          dispatch({ type: 'SET_ERROR', error: res.error })
        }
      } catch (err) {
        dispatch({ type: 'SET_ERROR', error: (err as Error).message })
      }
    },
    [state.repos]
  )

  // Auto-poll while any repo is in conflict/waiting/resolving state
  // so that external resolutions (e.g. manual git commit) are detected.
  const conflictPollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    const hasConflict = state.repos.some((r) => isConflictStatus(r.status))
    if (hasConflict && !conflictPollRef.current) {
      conflictPollRef.current = setInterval(async () => {
        try {
          const res = await engineApi.status()
          if (res.success) {
            dispatch({ type: 'SET_REPOS_SILENT', repos: res.data.repos ?? [] })
          }
        } catch {
          // silent — polling is best-effort
        }
      }, 5000)
    } else if (!hasConflict && conflictPollRef.current) {
      clearInterval(conflictPollRef.current)
      conflictPollRef.current = null
    }
  }, [state.repos])

  const markStartupSyncDone = useCallback(() => {
    startupSyncDoneRef.current = true
  }, [])

  const updateRepoStatus = useCallback((repoId: string, status: Repo['status']) => {
    dispatch({ type: 'SET_REPO_STATUS', repoId, status })
  }, [])

  const updateRepo = useCallback((updatedRepo: Repo) => {
    dispatch({ type: 'UPDATE_REPO', repo: updatedRepo })
  }, [])

  return (
    <RepoContext.Provider
      value={{ ...state, refresh, syncAll, syncRepo, scan, addRepo, removeRepo, updateRepoStatus, updateRepo, startupSyncDone: startupSyncDoneRef.current, markStartupSyncDone }}
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
