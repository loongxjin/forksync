import './assets/main.css'
import { useEffect } from 'react'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { RepoProvider } from './contexts/RepoContext'
import { AgentProvider } from './contexts/AgentContext'
import { SettingsProvider } from './contexts/SettingsContext'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Repos } from './pages/Repos'
import { Conflicts } from './pages/Conflicts'
import { ConflictDetail } from './pages/ConflictDetail'
import { Settings } from './pages/Settings'

function App(): JSX.Element {
  // Initialize theme from localStorage (default: dark)
  useEffect(() => {
    const saved = localStorage.getItem('forksync-theme')
    if (saved === 'light') {
      document.documentElement.classList.remove('dark')
    } else {
      document.documentElement.classList.add('dark')
    }
  }, [])

  return (
    <SettingsProvider>
      <RepoProvider>
        <AgentProvider>
          <HashRouter>
            <Routes>
              <Route element={<Layout />}>
                <Route path="/" element={<Dashboard />} />
                <Route path="/repos" element={<Repos />} />
                <Route path="/conflicts" element={<Conflicts />} />
                <Route path="/conflicts/:repoId" element={<ConflictDetail />} />
                <Route path="/settings" element={<Settings />} />
              </Route>
            </Routes>
          </HashRouter>
        </AgentProvider>
      </RepoProvider>
    </SettingsProvider>
  )
}

export default App
