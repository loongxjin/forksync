import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { engineApi } from '@/lib/api'
import type { Repo, PostSyncCommand } from '@shared/types/engine'
import { ArrowRight } from 'lucide-react'

interface RepoDetailPanelProps {
  repo: Repo
  onEditCommands: () => void
  /** Incremented by parent when commands may have changed externally */
  commandsVersion?: number
}

export function RepoDetailPanel({ repo, onEditCommands, commandsVersion }: RepoDetailPanelProps): JSX.Element {
  const { t } = useTranslation()
  const [commands, setCommands] = useState<PostSyncCommand[]>([])
  const [loading, setLoading] = useState(false)

  const loadCommands = useCallback(async () => {
    setLoading(true)
    try {
      const res = await engineApi.postSyncList(repo.name)
      if (res.success) {
        setCommands(res.data.commands ?? [])
      }
    } catch {
      // silent
    } finally {
      setLoading(false)
    }
  }, [repo.name])

  useEffect(() => {
    loadCommands()
  }, [loadCommands, commandsVersion])

  return (
    <div className="px-4 pb-4 space-y-4">
      <div className="border-t border-border pt-4 space-y-4">

        {/* Branch Mapping (read-only) */}
        <div>
          <p className="text-xs font-medium text-muted-foreground mb-1">{t('repos.branchMapping')}</p>
          {repo.branchMapping?.localBranch && repo.branchMapping?.remoteBranch ? (
            <div className="flex items-center gap-1.5 text-sm">
              <code className="rounded bg-secondary px-1.5 py-0.5 text-xs font-mono">{repo.branchMapping.localBranch}</code>
              <ArrowRight size={12} className="text-muted-foreground" />
              <code className="rounded bg-secondary px-1.5 py-0.5 text-xs font-mono">{repo.branchMapping.remoteBranch}</code>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              {repo.branch}{' '}
              <ArrowRight size={10} className="inline text-muted-foreground" />{' '}
              {repo.branch} <span className="text-muted-foreground">({t('addRepo.branchMappingHint')})</span>
            </p>
          )}
        </div>

        {/* Post-Sync Commands (read-only) */}
        <div>
          <p className="text-xs font-medium text-muted-foreground mb-1">
            {t('home.postSyncCommands')} ({commands.length})
          </p>

          {loading ? (
            <p className="text-xs text-muted-foreground">{t('common.loading')}</p>
          ) : commands.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('postSync.empty')}</p>
          ) : (
            <div className="space-y-1">
              {commands.map((cmd) => (
                <div key={cmd.id} className="flex items-center gap-2 text-sm px-2 py-1">
                  <span className="text-muted-foreground">$</span>
                  <code className="text-xs font-mono text-foreground">{cmd.cmd}</code>
                </div>
              ))}
            </div>
          )}
        </div>

      </div>
    </div>
  )
}
