import { Outlet, useNavigate } from 'react-router-dom'
import { useEffect } from 'react'
import { Sidebar } from './Sidebar'
import { engineApi } from '@/lib/api'

function TitleBar(): JSX.Element {
  return (
    <div className="titlebar flex h-[38px] shrink-0 items-center border-b border-border bg-card pl-20">
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
