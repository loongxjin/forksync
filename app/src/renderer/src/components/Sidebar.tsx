import { NavLink, useLocation } from 'react-router-dom'

const navItems = [
  { to: '/', label: 'Dashboard', icon: '🏠' },
  { to: '/repos', label: 'Repos', icon: '📦' },
  { to: '/conflicts', label: 'Conflicts', icon: '⚡' },
  { to: '/settings', label: 'Settings', icon: '⚙️' }
] as const

export function Sidebar(): JSX.Element {
  const location = useLocation()

  return (
    <aside className="flex h-screen w-60 flex-col border-r border-border bg-card">
      {/* Logo */}
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <span className="text-lg">🔄</span>
        <h1 className="text-base font-semibold text-foreground">ForkSync</h1>
      </div>

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
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>

      {/* Theme toggle */}
      <div className="border-t border-border px-4 py-3">
        <ThemeToggle />
      </div>
    </aside>
  )
}

function ThemeToggle(): JSX.Element {
  const toggleTheme = (): void => {
    const html = document.documentElement
    const isDark = html.classList.contains('dark')
    html.classList.toggle('dark', !isDark)
    localStorage.setItem('forksync-theme', isDark ? 'light' : 'dark')
  }

  return (
    <button
      onClick={toggleTheme}
      className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
    >
      <span className="text-base">🌙</span>
      <span>Toggle Theme</span>
    </button>
  )
}
