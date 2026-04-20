/**
 * HistoryContext — manages sync history state globally
 * so it survives page navigation.
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  type ReactNode
} from 'react'
import type { SyncHistoryRecord } from '@/types/engine'
import { engineApi } from '@/lib/api'

interface HistoryState {
  records: SyncHistoryRecord[]
  loading: boolean
  initialized: boolean
  lastLoadAt: number
}

type HistoryAction =
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_RECORDS'; records: SyncHistoryRecord[] }
  | { type: 'CLEAR' }
  | { type: 'SET_ERROR' }
  | { type: 'UPDATE_RECORD'; repoName: string; updates: Partial<SyncHistoryRecord> }

const initialState: HistoryState = {
  records: [],
  loading: false,
  initialized: false,
  lastLoadAt: 0
}

function historyReducer(state: HistoryState, action: HistoryAction): HistoryState {
  switch (action.type) {
    case 'SET_LOADING':
      return { ...state, loading: action.loading }
    case 'SET_RECORDS':
      return {
        ...state,
        records: action.records,
        loading: false,
        initialized: true,
        lastLoadAt: Date.now()
      }
    case 'CLEAR':
      return { ...state, records: [], initialized: false }
    case 'UPDATE_RECORD':
      return {
        ...state,
        records: state.records.map((r) =>
          r.repoName === action.repoName
            ? { ...r, ...action.updates }
            : r
        )
      }
    case 'SET_ERROR':
      return { ...state, loading: false }
    default:
      return state
  }
}

interface HistoryContextValue extends HistoryState {
  loadHistory: () => Promise<void>
  clearHistory: () => void
  updateRecord: (repoName: string, updates: Partial<SyncHistoryRecord>) => void
}

const HistoryContext = createContext<HistoryContextValue | null>(null)

export function HistoryProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(historyReducer, initialState)

  const loadHistory = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true })
    try {
      const res = await engineApi.history(undefined, 20)
      if (res.success && res.data) {
        dispatch({ type: 'SET_RECORDS', records: res.data.records ?? [] })
      } else {
        dispatch({ type: 'SET_ERROR' })
      }
    } catch {
      dispatch({ type: 'SET_ERROR' })
    }
  }, [])

  const clearHistory = useCallback(() => {
    dispatch({ type: 'CLEAR' })
  }, [])

  const updateRecord = useCallback((repoName: string, updates: Partial<SyncHistoryRecord>) => {
    dispatch({ type: 'UPDATE_RECORD', repoName, updates })
  }, [])

  return (
    <HistoryContext.Provider value={{ ...state, loadHistory, clearHistory, updateRecord }}>
      {children}
    </HistoryContext.Provider>
  )
}

export function useHistory(): HistoryContextValue {
  const ctx = useContext(HistoryContext)
  if (!ctx) {
    throw new Error('useHistory must be used within a HistoryProvider')
  }
  return ctx
}
