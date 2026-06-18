import './assets/main.css'
import './i18n'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { RepoProvider } from './contexts/RepoContext'
import { AgentProvider } from './contexts/AgentContext'
import { SettingsProvider } from './contexts/SettingsContext'
import { HistoryProvider } from './contexts/HistoryContext'
import { ToastProvider } from './contexts/ToastContext'
import { ErrorBoundary } from './components/ErrorBoundary'
import { Layout } from './components/Layout'
import { HomePage } from './pages/HomePage'
import { SettingsPage } from './pages/SettingsPage'
import { useEngineEvents } from './hooks/useEngineEvents'

/**
 * AppContent lives inside all providers so it can call useRepos / useHistory.
 * It opens the single /stream/events WebSocket that drives push-based
 * refreshes (replacing the renderer's status/history polling).
 */
function AppContent(): JSX.Element {
  useEngineEvents()
  return (
    <HashRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={
            <ErrorBoundary>
              <HomePage />
            </ErrorBoundary>
          } />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}

function App(): JSX.Element {
  return (
    <ErrorBoundary>
      <SettingsProvider>
        <ToastProvider>
          <HistoryProvider>
          <RepoProvider>
          <AgentProvider>
                <AppContent />
          </AgentProvider>
          </RepoProvider>
          </HistoryProvider>
        </ToastProvider>
      </SettingsProvider>
    </ErrorBoundary>
  )
}

export default App
