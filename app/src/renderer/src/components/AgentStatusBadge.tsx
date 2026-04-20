import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Bot } from 'lucide-react'
import type { AgentInfo } from '@/types/engine'

interface AgentStatusBadgeProps {
  agents: AgentInfo[]
  preferred: string
}

export function AgentStatusBadge({ agents, preferred }: AgentStatusBadgeProps): JSX.Element {
  const { t } = useTranslation()
  const preferredAgent = agents.find((a) => a.name === preferred)
  const installedCount = agents.filter((a) => a.installed).length

  if (installedCount === 0) {
    return (
      <div className="flex items-center gap-2">
        <Bot size={16} className="text-muted-foreground" />
        <span className="text-sm text-muted-foreground">{t('agentBadge.noAgentsInstalled')}</span>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      <Bot size={16} className="text-primary" />
      <span className="text-sm font-medium">
        {preferredAgent?.name ?? agents[0]?.name}
      </span>
      {preferredAgent?.version && (
        <span className="text-xs text-muted-foreground">{preferredAgent.version}</span>
      )}
      <Badge variant="success">{t('agentBadge.connected')}</Badge>
      {installedCount > 1 && (
        <span className="text-xs text-muted-foreground">
          {t('agentBadge.more', { count: installedCount - 1 })}
        </span>
      )}
    </div>
  )
}
