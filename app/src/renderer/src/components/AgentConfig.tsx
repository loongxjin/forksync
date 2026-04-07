import { useEffect } from 'react'
import { useAgents } from '@/contexts/AgentContext'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'

export function AgentConfig(): JSX.Element {
  const { agents, preferred, sessions, refreshAgents, refreshSessions, cleanup } = useAgents()

  useEffect(() => {
    refreshAgents()
    refreshSessions()
  }, [refreshAgents, refreshSessions])

  const handleCleanup = async (): Promise<void> => {
    await cleanup()
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <h3 className="text-sm font-medium">Detected Agents</h3>
        <div className="space-y-2">
          {agents.map((agent) => (
            <div
              key={agent.name}
              className="flex items-center justify-between rounded-md border border-border bg-card p-3"
            >
              <div className="flex items-center gap-2">
                <span>{agent.installed ? '✅' : '❌'}</span>
                <span className="text-sm font-medium">{agent.name}</span>
                {agent.version && (
                  <span className="text-xs text-muted-foreground">v{agent.version}</span>
                )}
                {agent.name === preferred && <Badge variant="info">preferred</Badge>}
              </div>
              <div className="text-xs text-muted-foreground">
                {agent.installed ? agent.path : 'not installed'}
              </div>
            </div>
          ))}
          {agents.length === 0 && (
            <p className="text-sm text-muted-foreground">No agent CLIs detected.</p>
          )}
        </div>
      </div>

      <Separator />

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">
            Sessions ({sessions.length})
          </h3>
          <Button variant="outline" size="sm" onClick={handleCleanup}>
            🧹 Cleanup Expired
          </Button>
        </div>
        {sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground">No active sessions.</p>
        ) : (
          <div className="space-y-1">
            {sessions.map((s) => (
              <div
                key={s.id}
                className="flex items-center justify-between rounded-md border border-border bg-card p-2 text-xs"
              >
                <div className="flex items-center gap-2">
                  <span>
                    {s.status === 'active' ? '🟢' : s.status === 'expired' ? '⏰' : '❌'}
                  </span>
                  <span className="font-medium">{s.agentName}</span>
                  <span className="text-muted-foreground">{s.repoId}</span>
                </div>
                <span className="text-muted-foreground">{s.status}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
