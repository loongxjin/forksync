/**
 * SettingsContext — manages app settings (theme, IDE config, etc.)
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  useEffect,
  type ReactNode
} from 'react'
import { engineApi } from '@/lib/api'
import type { IDEConfig, IDEInfo } from '@/types/ide'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

type Theme = 'dark' | 'light' | 'system'

interface SettingsState {
  theme: Theme
  ideConfig: IDEConfig | null
  ideLoading: boolean
}

type SettingsAction =
  | { type: 'SET_THEME'; theme: Theme }
  | { type: 'SET_IDE_CONFIG'; config: IDEConfig }
  | { type: 'SET_IDE_LOADING'; loading: boolean }

const initialState: SettingsState = {
  theme: (localStorage.getItem('forksync-theme') as Theme) || 'dark',
  ideConfig: null,
  ideLoading: true
}

function settingsReducer(state: SettingsState, action: SettingsAction): SettingsState {
  switch (action.type) {
    case 'SET_THEME':
      return { ...state, theme: action.theme }
    case 'SET_IDE_CONFIG':
      return { ...state, ideConfig: action.config, ideLoading: false }
    case 'SET_IDE_LOADING':
      return { ...state, ideLoading: action.loading }
    default:
      return state
  }
}

// ---------------------------------------------------------------------------
// Context interface
// ---------------------------------------------------------------------------

interface SettingsContextValue extends SettingsState {
  setTheme: (theme: Theme) => void
  refreshIDEConfig: () => Promise<void>
  setDefaultIDE: (ideId: string | null) => Promise<void>
  openInIDE: (repoPath: string, ideId: string) => Promise<{ success: boolean; error?: string }>
  addCustomIDE: (name: string, cliCommand: string) => Promise<void>
  removeCustomIDE: (ideId: string) => Promise<void>
  getInstalledIDEs: () => IDEInfo[]
  getDefaultIDE: () => IDEInfo | null
}

const SettingsContext = createContext<SettingsContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function SettingsProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(settingsReducer, initialState)

  const setTheme = useCallback((theme: Theme) => {
    dispatch({ type: 'SET_THEME', theme })
    localStorage.setItem('forksync-theme', theme)

    const html = document.documentElement
    if (theme === 'dark') {
      html.classList.add('dark')
    } else if (theme === 'light') {
      html.classList.remove('dark')
    } else {
      // system
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
      html.classList.toggle('dark', prefersDark)
    }
  }, [])

  const refreshIDEConfig = useCallback(async () => {
    dispatch({ type: 'SET_IDE_LOADING', loading: true })
    try {
      const config = await engineApi.ideGetConfig()
      dispatch({ type: 'SET_IDE_CONFIG', config })
    } catch {
      dispatch({ type: 'SET_IDE_LOADING', loading: false })
    }
  }, [])

  const setDefaultIDE = useCallback(async (ideId: string | null) => {
    await engineApi.ideSetDefault(ideId)
    await refreshIDEConfig()
  }, [refreshIDEConfig])

  const openInIDE = useCallback(
    async (repoPath: string, ideId: string) => {
      return engineApi.ideOpen(repoPath, ideId)
    },
    []
  )

  const addCustomIDE = useCallback(
    async (name: string, cliCommand: string) => {
      await engineApi.ideAddCustom(name, cliCommand)
      await refreshIDEConfig()
    },
    [refreshIDEConfig]
  )

  const removeCustomIDE = useCallback(
    async (ideId: string) => {
      await engineApi.ideRemoveCustom(ideId)
      await refreshIDEConfig()
    },
    [refreshIDEConfig]
  )

  const getInstalledIDEs = useCallback((): IDEInfo[] => {
    return state.ideConfig?.detectedIDEs.filter((ide) => ide.installed) ?? []
  }, [state.ideConfig])

  const getDefaultIDE = useCallback((): IDEInfo | null => {
    if (!state.ideConfig?.defaultIDE) return null
    return (
      state.ideConfig.detectedIDEs.find((ide) => ide.id === state.ideConfig!.defaultIDE) ?? null
    )
  }, [state.ideConfig])

  // Load IDE config on mount
  useEffect(() => {
    refreshIDEConfig()
  }, [refreshIDEConfig])

  return (
    <SettingsContext.Provider
      value={{
        ...state,
        setTheme,
        refreshIDEConfig,
        setDefaultIDE,
        openInIDE,
        addCustomIDE,
        removeCustomIDE,
        getInstalledIDEs,
        getDefaultIDE
      }}
    >
      {children}
    </SettingsContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useSettings(): SettingsContextValue {
  const ctx = useContext(SettingsContext)
  if (!ctx) {
    throw new Error('useSettings must be used within a SettingsProvider')
  }
  return ctx
}
