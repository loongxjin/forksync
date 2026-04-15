import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useRepos } from '@/contexts/RepoContext'
import { RepoStatusBadge } from '@/components/RepoRow'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { Repo } from '@/types/engine'

export function Conflicts(): JSX.Element {
  const { t } = useTranslation()
  const { repos, loading, initialized, refresh } = useRepos()
  const navigate = useNavigate()

  useEffect(() => {
    if (!initialized) {
      refresh()
    }
  }, [initialized, refresh])

  // Filter repos with conflict-related statuses
  const conflictRepos = repos.filter(
    (r) => r.status === 'conflict' || r.status === 'resolving' || r.status === 'resolved'
  )

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('conflicts.title')}</h2>
        <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
          <span className={loading ? 'inline-block animate-spin' : ''}>
            {loading ? '⟳' : '🔄'}
          </span>{' '}
          {t('conflicts.refresh')}
        </Button>
      </div>

      {conflictRepos.length === 0 ? (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">{t('conflicts.noConflicts')}</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('conflicts.noConflictsHint')}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {conflictRepos.map((repo) => (
            <ConflictRow
              key={repo.id}
              repo={repo}
              onClick={() => navigate(`/conflicts/${repo.name}`)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ConflictRow({ repo, onClick }: { repo: Repo; onClick: () => void }): JSX.Element {
  const { t } = useTranslation()
  return (
    <button
      onClick={onClick}
      className="flex w-full items-center justify-between rounded-lg border border-border bg-card p-4 text-left transition-colors hover:bg-accent/30"
    >
      <div className="flex items-center gap-3">
        <RepoStatusBadge status={repo.status} />
        <span className="font-medium">{repo.name}</span>
        {repo.behindBy > 0 && (
          <Badge variant="muted">{t('repos.behind', { count: repo.behindBy })}</Badge>
        )}
      </div>
      <div className="flex items-center gap-2">
        {repo.errorMessage && (
          <span className="max-w-48 truncate text-xs text-red-400">{repo.errorMessage}</span>
        )}
        <span className="text-xs text-muted-foreground">→</span>
      </div>
    </button>
  )
}
