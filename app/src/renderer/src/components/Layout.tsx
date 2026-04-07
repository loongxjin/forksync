import { Outlet, useNavigate } from 'react-router-dom'
import { useEffect } from 'react'
import { Sidebar } from './Sidebar'
import { engineApi } from '@/lib/api'

function TitleBar(): JSX.Element {
  return (
    <div className="titlebar flex h-[38px] shrink-0 items-center border-b border-border bg-card pl-20">
      <span className="text-sm">🔄</span>
      <h1 className="ml-1.5 text-sm font-semibold text-foreground">ForkSync</h1>
    </div>
  )
}

export function Layout(): JSX.Element {
  const navigate = useNavigate()

  // Listen for navigation events from main process (notification click-through)
  useEffect(() => {
    const unsubscribe = engineApi.onNavigate?.((path: string) => {
      navigate(path)
    })
    return () => {
      unsubscribe?.()
    }
  }, [navigate])

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <TitleBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <main className="flex-1 overflow-y-auto">
          <div className="mx-auto max-w-5xl p-6">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
