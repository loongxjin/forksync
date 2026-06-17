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

function App(): JSX.Element {
  return (
    <ErrorBoundary>
      <SettingsProvider>
        <ToastProvider>
          <HistoryProvider>
          <RepoProvider>
          <AgentProvider>
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
          </AgentProvider>
          </RepoProvider>
          </HistoryProvider>
        </ToastProvider>
      </SettingsProvider>
    </ErrorBoundary>
  )
}

export default App
