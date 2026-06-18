import { Inbox } from 'lucide-react'

interface EmptyStateProps {
  icon?: React.ReactNode
  title: string
  description?: string
  action?: React.ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps): JSX.Element {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-secondary">
        {icon ?? <Inbox size={24} className="text-muted-foreground" />}
      </div>
      <p className="mt-4 text-sm font-semibold text-foreground">{title}</p>
      {description && (
        <p className="mt-1.5 max-w-md text-sm text-muted-foreground leading-relaxed">{description}</p>
      )}
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}
