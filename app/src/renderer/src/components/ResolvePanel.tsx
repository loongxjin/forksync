import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

interface ResolvePanelProps {
  repoName: string
  status: 'conflict' | 'resolving' | 'resolved'
  agentName?: string
  summary?: string
  resolvedFiles?: string[]
  onResolve: (agent?: string) => Promise<void>
  onAccept: () => Promise<void>
  onReject: () => Promise<void>
  loading: boolean
}

export function ResolvePanel({
  repoName,
  status,
  agentName,
  summary,
  resolvedFiles,
  onResolve,
  onAccept,
  onReject,
  loading
}: ResolvePanelProps): JSX.Element {
  const [resolving, setResolving] = useState(false)

  const handleResolve = async (): Promise<void> => {
    setResolving(true)
    try {
      await onResolve()
    } finally {
      setResolving(false)
    }
  }

  return (
    <div className="rounded-lg border border-border bg-card p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">Resolve Actions</h3>
        <Badge
          variant={
            status === 'resolved'
              ? 'success'
              : status === 'resolving'
                ? 'warning'
                : 'error'
          }
        >
          {status}
        </Badge>
      </div>

      {agentName && (
        <p className="text-xs text-muted-foreground">
          Agent: <span className="text-foreground">{agentName}</span>
        </p>
      )}

      {summary && (
        <div className="rounded-md bg-accent/30 p-3">
          <p className="text-xs font-medium text-muted-foreground">Agent Summary</p>
          <p className="mt-1 text-sm">{summary}</p>
        </div>
      )}

      {resolvedFiles && resolvedFiles.length > 0 && (
        <div>
          <p className="text-xs font-medium text-muted-foreground">Resolved Files</p>
          <ul className="mt-1 space-y-0.5">
            {resolvedFiles.map((f) => (
              <li key={f} className="text-xs text-emerald-400">
                ✓ {f}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="flex gap-2">
        {status === 'conflict' && (
          <Button onClick={handleResolve} disabled={loading || resolving} size="sm">
            {resolving ? 'Resolving...' : '⚡ Resolve with Agent'}
          </Button>
        )}
        {status === 'resolved' && (
          <>
            <Button onClick={onAccept} disabled={loading} size="sm" variant="default">
              ✓ Accept
            </Button>
            <Button onClick={onReject} disabled={loading} size="sm" variant="destructive">
              ✗ Reject & Rollback
            </Button>
          </>
        )}
        {status === 'resolving' && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span className="animate-pulse">🟠</span>
            Agent is resolving conflicts for {repoName}...
          </div>
        )}
      </div>
    </div>
  )
}
