import { Badge } from '@/components/ui/badge'
import type { AgentInfo } from '@/types/engine'

interface AgentStatusBadgeProps {
  agents: AgentInfo[]
  preferred: string
}

export function AgentStatusBadge({ agents, preferred }: AgentStatusBadgeProps): JSX.Element {
  const preferredAgent = agents.find((a) => a.name === preferred)
  const installedCount = agents.filter((a) => a.installed).length

  if (installedCount === 0) {
    return (
      <div className="flex items-center gap-2">
        <span className="text-sm">🤖</span>
        <span className="text-sm text-muted-foreground">No agents installed</span>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      <span className="text-sm">🤖</span>
      <span className="text-sm font-medium">
        {preferredAgent?.name ?? agents[0]?.name}
      </span>
      {preferredAgent?.version && (
        <span className="text-xs text-muted-foreground">{preferredAgent.version}</span>
      )}
      <Badge variant="success">● Connected</Badge>
      {installedCount > 1 && (
        <span className="text-xs text-muted-foreground">
          +{installedCount - 1} more
        </span>
      )}
    </div>
  )
}
