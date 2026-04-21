import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { engineApi } from '@/lib/api'
import type { Repo, PostSyncCommand } from '@/types/engine'

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
      <div className="border-t border-border pt-4">
        {/* Post-Sync Commands */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <p className="text-xs font-medium text-muted-foreground">
              {t('home.postSyncCommands')} ({commands.length})
            </p>
            <button
              onClick={onEditCommands}
              className="text-xs text-primary hover:underline"
            >
              {commands.length > 0 ? t('home.editCommands') : t('home.addCommand')}
            </button>
          </div>

          {loading ? (
            <p className="text-xs text-muted-foreground">{t('common.loading')}</p>
          ) : commands.length === 0 ? (
            <button
              onClick={onEditCommands}
              className="text-sm text-muted-foreground hover:text-foreground transition-colors"
            >
              {t('home.addCommand')}
            </button>
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
