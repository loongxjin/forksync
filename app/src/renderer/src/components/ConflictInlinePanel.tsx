import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DiffViewer } from '@/components/DiffViewer'
import type { Repo, ResolveData } from '@/types/engine'

interface ConflictInlinePanelProps {
  repo: Repo
  resolveResult: ResolveData | null
  onResolve: () => Promise<void>
  onAccept: () => Promise<void>
  onReject: () => Promise<void>
  loading: boolean
}

export function ConflictInlinePanel({
  repo,
  resolveResult,
  onResolve,
  onAccept,
  onReject,
  loading
}: ConflictInlinePanelProps): JSX.Element {
  const { t } = useTranslation()
  const [resolving, setResolving] = useState(false)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)

  const handleResolve = async (): Promise<void> => {
    setResolving(true)
    try {
      await onResolve()
    } finally {
      setResolving(false)
    }
  }

  const agentResult = resolveResult?.agentResult
  const conflicts = resolveResult?.conflicts ?? []
  const diff = agentResult?.diff
  const status = repo.status

  return (
    <div className="px-4 pb-4 space-y-4">
      <div className="border-t border-border pt-4">
        {/* Agent info */}
        {agentResult?.agentName && (
          <p className="text-xs text-muted-foreground mb-2">
            🤖 {t('resolvePanel.agent')} <span className="text-foreground font-medium">{agentResult.agentName}</span>
          </p>
        )}

        {/* AI Summary */}
        {agentResult?.summary && (
          <div className="rounded-md bg-accent/30 p-3 mb-3">
            <p className="text-xs font-medium text-muted-foreground mb-1">
              📝 {t('home.aiSuggestion')}
            </p>
            <p className="text-sm">{agentResult.summary}</p>
          </div>
        )}

        {/* Conflict files */}
        {conflicts.length > 0 && (
          <div className="mb-3">
            <p className="text-xs font-medium text-muted-foreground mb-1">
              {t('conflicts.conflictFiles')}
            </p>
            <div className="space-y-1">
              {conflicts.map((f) => (
                <button
                  key={f.path}
                  onClick={() => setSelectedFile(selectedFile === f.path ? null : f.path)}
                  className={`flex items-center gap-2 text-sm w-full text-left px-2 py-1 rounded transition-colors ${
                    selectedFile === f.path ? 'bg-accent/50' : 'hover:bg-accent/30'
                  }`}
                >
                  <span className="text-red-400">⚠</span>
                  <span className="truncate">{f.path}</span>
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Diff preview */}
        {diff && selectedFile && (
          <div className="mb-3">
            <p className="text-xs font-medium text-muted-foreground mb-1">
              {t('conflicts.diffPreview')} — {selectedFile}
            </p>
            <DiffViewer diff={diff} className="max-h-64" />
          </div>
        )}

        {/* Action buttons */}
        <div className="flex gap-2">
          {status === 'conflict' && (
            <Button onClick={handleResolve} disabled={loading || resolving} size="sm">
              {resolving ? t('resolvePanel.resolving') : t('resolvePanel.resolveWithAgent')}
            </Button>
          )}
          {status === 'resolved' && (
            <>
              <Button onClick={onAccept} disabled={loading} size="sm" variant="default">
                ✅ {t('resolvePanel.accept')}
              </Button>
              <Button onClick={onReject} disabled={loading} size="sm" variant="destructive">
                ❌ {t('resolvePanel.rejectRollback')}
              </Button>
            </>
          )}
          {status === 'resolving' && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span className="animate-pulse">🟠</span>
              {t('resolvePanel.resolvingStatus', { repoName: repo.name })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
