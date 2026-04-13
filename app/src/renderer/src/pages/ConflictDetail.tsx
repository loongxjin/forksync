import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { ResolvePanel } from '@/components/ResolvePanel'
import { DiffViewer } from '@/components/DiffViewer'
import { Button } from '@/components/ui/button'
import type { ResolveData } from '@/types/engine'

export function ConflictDetail(): JSX.Element {
  const { repoId } = useParams<{ repoId: string }>()
  const navigate = useNavigate()
  const { repos, initialized, refresh, updateRepoStatus } = useRepos()
  const { resolve, resolveAccept, resolveReject, preferred, loading, error: agentError } = useAgents()

  const [resolveResult, setResolveResult] = useState<ResolveData | null>(null)
  const [localLoading, setLocalLoading] = useState(false)

  const repo = repos.find((r) => r.name === repoId || r.id === repoId)

  useEffect(() => {
    if (!initialized) {
      refresh()
    }
  }, [initialized, refresh])

  const handleResolve = async (): Promise<void> => {
    if (!repoId || !repo) return
    // Optimistically update UI to "resolving" before the blocking agent call
    updateRepoStatus(repo.id, 'resolving')
    setLocalLoading(true)
    try {
      const result = await resolve(repoId, { agent: preferred || undefined })
      if (result) {
        setResolveResult(result)
      }
      await refresh()
    } finally {
      setLocalLoading(false)
    }
  }

  const handleAccept = async (): Promise<void> => {
    if (!repoId) return
    setLocalLoading(true)
    try {
      const result = await resolveAccept(repoId)
      if (!result) {
        // resolveAccept returned null — error already dispatched in AgentContext
        // Don't navigate away so the user can see the error
        return
      }
      await refresh()
      navigate('/conflicts')
    } finally {
      setLocalLoading(false)
    }
  }

  const handleReject = async (): Promise<void> => {
    if (!repoId) return
    setLocalLoading(true)
    try {
      const ok = await resolveReject(repoId)
      if (!ok) {
        // resolveReject returned false — error already dispatched in AgentContext
        return
      }
      setResolveResult(null)
      await refresh()
    } finally {
      setLocalLoading(false)
    }
  }

  if (!repo) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" size="sm" onClick={() => navigate('/conflicts')}>
          ← Back
        </Button>
        <p className="text-sm text-muted-foreground">Repository not found.</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" onClick={() => navigate('/conflicts')}>
          ← Back
        </Button>
        <h2 className="text-xl font-semibold">
          {repo.name}
          <span className="ml-2 text-sm font-normal text-muted-foreground">Conflict Resolution</span>
        </h2>
      </div>

      <ResolvePanel
        repoName={repo.name}
        status={
          repo.status === 'conflict' || repo.status === 'resolving' || repo.status === 'resolved'
            ? repo.status
            : 'conflict'
        }
        agentName={resolveResult?.agentResult?.sessionId ? preferred : undefined}
        summary={resolveResult?.agentResult?.summary}
        resolvedFiles={resolveResult?.agentResult?.resolvedFiles}
        onResolve={handleResolve}
        onAccept={handleAccept}
        onReject={handleReject}
        loading={loading || localLoading}
      />

      {/* Error display */}
      {agentError && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3">
          <p className="text-sm text-red-400">⚠ {agentError}</p>
        </div>
      )}

      {/* Diff Preview */}
      {resolveResult?.agentResult?.diff && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground">Diff Preview</h3>
          <DiffViewer diff={resolveResult.agentResult.diff} className="max-h-96" />
        </div>
      )}

      {/* Conflict files list */}
      {(resolveResult?.conflicts ?? repo.status === 'conflict') && !resolveResult && (
        <div className="rounded-lg border border-border bg-card p-4">
          <h3 className="text-sm font-medium text-muted-foreground">Conflict Files</h3>
          {resolveResult?.conflicts && resolveResult.conflicts.length > 0 ? (
            <ul className="mt-2 space-y-1">
              {resolveResult.conflicts.map((f) => (
                <li key={f.path} className="text-sm text-red-400">
                  ⚠ {f.path}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-2 text-sm text-muted-foreground">
              Run resolve to detect conflict files.
            </p>
          )}
        </div>
      )}
    </div>
  )
}
