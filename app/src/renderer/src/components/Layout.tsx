import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'

export function Layout(): JSX.Element {
  return (
    <div className="flex h-screen bg-background text-foreground">
      <Sidebar />
      <main className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-5xl p-6">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
