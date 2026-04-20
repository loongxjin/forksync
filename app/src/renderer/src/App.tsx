import './assets/main.css'
import './i18n'
import { useEffect } from 'react'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { RepoProvider } from './contexts/RepoContext'
import { AgentProvider } from './contexts/AgentContext'
import { SettingsProvider } from './contexts/SettingsContext'
import { HistoryProvider } from './contexts/HistoryContext'
import { SettingsDrawerProvider } from './contexts/SettingsDrawerContext'
import { ErrorBoundary } from './components/ErrorBoundary'
import { Layout } from './components/Layout'
import { HomePage } from './pages/HomePage'

function App(): JSX.Element {
  useEffect(() => {
    const saved = localStorage.getItem('forksync-theme')
    if (saved === 'light') {
      document.documentElement.classList.remove('dark')
    } else {
      document.documentElement.classList.add('dark')
    }
  }, [])

  return (
    <ErrorBoundary>
      <SettingsProvider>
        <RepoProvider>
          <AgentProvider>
            <HistoryProvider>
              <SettingsDrawerProvider>
                <HashRouter>
                  <Routes>
                    <Route element={<Layout />}>
                      <Route path="/" element={<HomePage />} />
                    </Route>
                  </Routes>
                </HashRouter>
              </SettingsDrawerProvider>
            </HistoryProvider>
          </AgentProvider>
        </RepoProvider>
      </SettingsProvider>
    </ErrorBoundary>
  )
}

export default App
