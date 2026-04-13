import { NavLink, useLocation } from 'react-router-dom'
import { useSettings } from '@/contexts/SettingsContext'
import { useTranslation } from 'react-i18next'
import i18n from 'i18next'

const navItems = [
  { to: '/', labelKey: 'nav.dashboard', icon: '🏠' },
  { to: '/repos', labelKey: 'nav.repos', icon: '📦' },
  { to: '/conflicts', labelKey: 'nav.conflicts', icon: '⚡' },
  { to: '/settings', labelKey: 'nav.settings', icon: '⚙️' }
] as const

export function Sidebar(): JSX.Element {
  const location = useLocation()
  const { t } = useTranslation()

  return (
    <aside className="flex h-full w-60 flex-col border-r border-border bg-card">
      {/* Navigation */}
      <nav className="flex-1 space-y-1 px-2 py-3">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={() => {
              const isActive =
                item.to === '/'
                  ? location.pathname === '/'
                  : location.pathname.startsWith(item.to)
              return `flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
              }`
            }}
          >
            <span className="text-base">{item.icon}</span>
            <span>{t(item.labelKey)}</span>
          </NavLink>
        ))}
      </nav>

      {/* Theme toggle + Language toggle */}
      <div className="border-t border-border px-4 py-3 space-y-1">
        <ThemeToggle />
        <LanguageToggle />
      </div>
    </aside>
  )
}

function ThemeToggle(): JSX.Element {
  const { theme, setTheme } = useSettings()
  const { t } = useTranslation()

  const cycleTheme = (): void => {
    const next = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark'
    setTheme(next)
  }

  const icon = theme === 'dark' ? '🌙' : theme === 'light' ? '☀️' : '💻'
  const label = theme === 'dark' ? t('theme.dark') : theme === 'light' ? t('theme.light') : t('theme.system')

  return (
    <button
      onClick={cycleTheme}
      className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
    >
      <span className="text-base">{icon}</span>
      <span>{label}</span>
    </button>
  )
}

function LanguageToggle(): JSX.Element {
  const { i18n } = useTranslation()

  const currentLang = i18n.language?.startsWith('zh') ? 'zh' : 'en'

  const changeLanguage = (lng: string) => {
    i18n.changeLanguage(lng)
    localStorage.setItem('forksync-locale', lng)
  }

  const toggleLanguage = (): void => {
    changeLanguage(currentLang === 'zh' ? 'en' : 'zh')
  }

  return (
    <button
      onClick={toggleLanguage}
      className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
    >
      <span className="text-base">🌐</span>
      <span>{currentLang === 'zh' ? '中文' : 'EN'}</span>
    </button>
  )
}
