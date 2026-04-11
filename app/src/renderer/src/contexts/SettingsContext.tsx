/**
 * SettingsContext — manages app settings (theme, IDE config, engine config)
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
import type { EngineConfig } from '@/types/engine'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

type Theme = 'dark' | 'light' | 'system'

interface SettingsState {
  theme: Theme
  ideConfig: IDEConfig | null
  ideLoading: boolean
  engineConfig: EngineConfig | null
  configLoading: boolean
  configError: string | null
}

type SettingsAction =
  | { type: 'SET_THEME'; theme: Theme }
  | { type: 'SET_IDE_CONFIG'; config: IDEConfig }
  | { type: 'SET_IDE_LOADING'; loading: boolean }
  | { type: 'SET_ENGINE_CONFIG'; config: EngineConfig }
  | { type: 'SET_CONFIG_LOADING'; loading: boolean }
  | { type: 'SET_CONFIG_ERROR'; error: string | null }

const initialState: SettingsState = {
  theme: (localStorage.getItem('forksync-theme') as Theme) || 'dark',
  ideConfig: null,
  ideLoading: true,
  engineConfig: null,
  configLoading: true,
  configError: null
}

function settingsReducer(state: SettingsState, action: SettingsAction): SettingsState {
  switch (action.type) {
    case 'SET_THEME':
      return { ...state, theme: action.theme }
    case 'SET_IDE_CONFIG':
      return { ...state, ideConfig: action.config, ideLoading: false }
    case 'SET_IDE_LOADING':
      return { ...state, ideLoading: action.loading }
    case 'SET_ENGINE_CONFIG':
      return { ...state, engineConfig: action.config, configLoading: false, configError: null }
    case 'SET_CONFIG_LOADING':
      return { ...state, configLoading: action.loading }
    case 'SET_CONFIG_ERROR':
      return { ...state, configError: action.error, configLoading: false }
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
  // Engine config
  refreshConfig: () => Promise<void>
  updateConfig: (key: string, value: string) => Promise<void>
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

  // --- Engine config methods ---

  const refreshConfig = useCallback(async () => {
    dispatch({ type: 'SET_CONFIG_LOADING', loading: true })
    try {
      const result = await engineApi.configGet()
      if (result.success && result.data) {
        dispatch({ type: 'SET_ENGINE_CONFIG', config: result.data })
      } else {
        dispatch({ type: 'SET_CONFIG_ERROR', error: result.error || 'Failed to load config' })
      }
    } catch (err) {
      dispatch({ type: 'SET_CONFIG_ERROR', error: err instanceof Error ? err.message : String(err) })
    }
  }, [])

  const updateConfig = useCallback(async (key: string, value: string) => {
    try {
      const result = await engineApi.configSet(key, value)
      if (result.success) {
        // Refresh full config after set
        await refreshConfig()
      } else {
        dispatch({ type: 'SET_CONFIG_ERROR', error: result.error || 'Failed to update config' })
      }
    } catch (err) {
      dispatch({ type: 'SET_CONFIG_ERROR', error: err instanceof Error ? err.message : String(err) })
    }
  }, [refreshConfig])

  // Load configs on mount
  useEffect(() => {
    refreshIDEConfig()
    refreshConfig()
  }, [refreshIDEConfig, refreshConfig])

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
        getDefaultIDE,
        refreshConfig,
        updateConfig
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
