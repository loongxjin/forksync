/**
 * SettingsContext — manages app settings (theme, etc.)
 */

import {
  createContext,
  useContext,
  useReducer,
  useCallback,
  type ReactNode
} from 'react'

// ---------------------------------------------------------------------------
// State & Actions
// ---------------------------------------------------------------------------

type Theme = 'dark' | 'light' | 'system'

interface SettingsState {
  theme: Theme
}

type SettingsAction = { type: 'SET_THEME'; theme: Theme }

const initialState: SettingsState = {
  theme: (localStorage.getItem('forksync-theme') as Theme) || 'dark'
}

function settingsReducer(state: SettingsState, action: SettingsAction): SettingsState {
  switch (action.type) {
    case 'SET_THEME':
      return { ...state, theme: action.theme }
    default:
      return state
  }
}

// ---------------------------------------------------------------------------
// Context interface
// ---------------------------------------------------------------------------

interface SettingsContextValue extends SettingsState {
  setTheme: (theme: Theme) => void
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

  return (
    <SettingsContext.Provider value={{ ...state, setTheme }}>
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
