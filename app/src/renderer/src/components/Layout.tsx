import { Outlet } from 'react-router-dom'
import { useEffect } from 'react'
import { engineApi } from '@/lib/api'
import { Toast } from './ui/toast'
import { useRepos } from '@/contexts/RepoContext'
import { useSettings } from '@/contexts/SettingsContext'
import { useSettingsDrawer } from '@/contexts/SettingsDrawerContext'
import { useTranslation } from 'react-i18next'
import i18n from 'i18next'
import { SettingsDrawer } from './SettingsDrawer'
import { Settings, Moon, Sun, Monitor, Languages } from 'lucide-react'

function TitleBar(): JSX.Element {
  const { theme, setTheme } = useSettings()
  const { t } = useTranslation()
  const { openDrawer } = useSettingsDrawer()

  const cycleTheme = (): void => {
    const next = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark'
    setTheme(next)
  }

  const ThemeIcon = theme === 'dark' ? Moon : theme === 'light' ? Sun : Monitor

  const currentLang = i18n.language?.startsWith('zh') ? 'zh' : 'en'
  const toggleLanguage = (): void => {
    const next = currentLang === 'zh' ? 'en' : 'zh'
    i18n.changeLanguage(next)
    localStorage.setItem('forksync-locale', next)
  }

  return (
    <div className="titlebar flex h-[38px] shrink-0 items-center justify-between border-b border-border bg-card pl-20 pr-4">
      <div className="flex items-center">
        <svg width="18" height="18" viewBox="0 0 512 512" xmlns="http://www.w3.org/2000/svg" className="shrink-0">
          <defs>
            <linearGradient id="bgT" x1="0" y1="0" x2="512" y2="512" gradientUnits="userSpaceOnUse">
              <stop offset="0%" stopColor="#1e3a5f"/>
              <stop offset="100%" stopColor="#0f172a"/>
            </linearGradient>
            <linearGradient id="g1T" x1="180" y1="120" x2="330" y2="400" gradientUnits="userSpaceOnUse">
              <stop offset="0%" stopColor="#60a5fa"/>
              <stop offset="100%" stopColor="#2563eb"/>
            </linearGradient>
            <linearGradient id="g2T" x1="180" y1="120" x2="330" y2="400" gradientUnits="userSpaceOnUse">
              <stop offset="0%" stopColor="#34d399"/>
              <stop offset="100%" stopColor="#059669"/>
            </linearGradient>
          </defs>
          <rect x="20" y="20" width="472" height="472" rx="108" fill="url(#bgT)"/>
          <rect x="21" y="21" width="470" height="470" rx="107" fill="none" stroke="white" strokeOpacity="0.07"/>
          <path d="M 175 340 A 130 130 0 0 1 175 170" fill="none" stroke="url(#g1T)" strokeWidth="36" strokeLinecap="round"/>
          <polygon points="210,135 193,188 157,152" fill="#3b82f6"/>
          <path d="M 337 172 A 130 130 0 0 1 337 342" fill="none" stroke="url(#g2T)" strokeWidth="36" strokeLinecap="round"/>
          <polygon points="302,377 319,324 355,360" fill="#059669"/>
          <circle cx="256" cy="256" r="14" fill="white" opacity="0.10"/>
          <circle cx="256" cy="256" r="7" fill="white" opacity="0.22"/>
        </svg>
        <h1 className="ml-2 text-sm font-semibold text-foreground tracking-tight">ForkSync</h1>
      </div>

      <div className="flex items-center gap-0.5">
        <button
          onClick={openDrawer}
          className="press-scale rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          title={t('nav.settings')}
        >
          <Settings size={16} />
        </button>
        <button
          onClick={cycleTheme}
          className="press-scale rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          title={theme === 'dark' ? t('theme.dark') : theme === 'light' ? t('theme.light') : t('theme.system')}
        >
          <ThemeIcon size={16} />
        </button>
        <button
          onClick={toggleLanguage}
          className="press-scale rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          title={t('nav.language')}
        >
          <Languages size={16} />
        </button>
      </div>
    </div>
  )
}

export function Layout(): JSX.Element {
  const { toast, hideToast } = useRepos()
  const { open, closeDrawer } = useSettingsDrawer()

  // Listen for navigation events from main process (notification click-through)
  // In single-page mode, we no longer navigate; just refresh repos
  useEffect(() => {
    const unsubscribe = engineApi.onNavigate?.(() => {
      // Single-page mode: navigation is no-op; user stays on home page
    })
    return () => {
      unsubscribe?.()
    }
  }, [])

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <TitleBar />
      <main className="flex-1 overflow-y-auto min-h-0">
        <div className="mx-auto max-w-5xl p-6">
          <Outlet />
        </div>
      </main>
      <Toast
        message={toast.message}
        visible={toast.visible}
        type={toast.type}
        onClose={hideToast}
        duration={2000}
      />
      <SettingsDrawer open={open} onOpenChange={closeDrawer} />
    </div>
  )
}
