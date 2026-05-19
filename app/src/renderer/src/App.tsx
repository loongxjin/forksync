import './assets/main.css'
import './i18n'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { RepoProvider } from './contexts/RepoContext'
import { AgentProvider } from './contexts/AgentContext'
import { SettingsProvider } from './contexts/SettingsContext'
import { HistoryProvider } from './contexts/HistoryContext'
import { SettingsDrawerProvider } from './contexts/SettingsDrawerContext'
import { ToastProvider } from './contexts/ToastContext'
import { ErrorBoundary } from './components/ErrorBoundary'
import { Layout } from './components/Layout'
import { HomePage } from './pages/HomePage'

function App(): JSX.Element {
  return (
    <ErrorBoundary>
      <SettingsProvider>
        <ToastProvider>
          <RepoProvider>
          <AgentProvider>
            <HistoryProvider>
              <SettingsDrawerProvider>
                <HashRouter>
                  <Routes>
                    <Route element={<Layout />}>
                      <Route path="/" element={
                        <ErrorBoundary>
                          <HomePage />
                        </ErrorBoundary>
                      } />
                    </Route>
                  </Routes>
                </HashRouter>
              </SettingsDrawerProvider>
            </HistoryProvider>
          </AgentProvider>
        </RepoProvider>
        </ToastProvider>
      </SettingsProvider>
    </ErrorBoundary>
  )
}

export default App
