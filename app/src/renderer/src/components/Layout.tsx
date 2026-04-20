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
            <linearGradient id="bgT" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#13162b"/>
              <stop offset="100%" stopColor="#0d1020"/>
            </linearGradient>
          </defs>
          <rect x="20" y="20" width="472" height="472" rx="100" fill="url(#bgT)"/>
          <path d="M 256 96 A 160 160 0 0 1 416 256" fill="none" stroke="#5b8ad6" strokeWidth="24" strokeLinecap="round"/>
          <polygon points="424,232 416,274 390,246" fill="#5b8ad6"/>
          <path d="M 256 416 A 160 160 0 0 1 96 256" fill="none" stroke="#e8a838" strokeWidth="24" strokeLinecap="round"/>
          <polygon points="88,280 96,238 122,266" fill="#e8a838"/>
          <path d="M 256 226 A 30 30 0 0 1 256 286" fill="none" stroke="#5b8ad6" strokeWidth="4" opacity="0.5"/>
          <path d="M 256 286 A 30 30 0 0 1 256 226" fill="none" stroke="#e8a838" strokeWidth="4" opacity="0.5"/>
          <circle cx="256" cy="256" r="14" fill="#5b8ad6" opacity="0.9"/>
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
