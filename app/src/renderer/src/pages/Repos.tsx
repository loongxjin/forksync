import { useEffect, useState, useCallback, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useRepos } from '@/contexts/RepoContext'
import { RepoRow } from '@/components/RepoRow'
import { AddRepoDialog } from '@/components/AddRepoDialog'
import { ScanDialog } from '@/components/ScanDialog'
import { RepoSettingsDialog } from '@/components/RepoSettingsDialog'
import { Button } from '@/components/ui/button'

export function Repos(): JSX.Element {
  const { t } = useTranslation()
  const { repos, scannedRepos, loading, initialized, error, refresh, syncRepo, scan, addRepo, removeRepo } =
    useRepos()
  const [showAdd, setShowAdd] = useState(false)
  const [showScan, setShowScan] = useState(false)
  const [settingsRepo, setSettingsRepo] = useState<string | null>(null)
  const [scanInitialDir, setScanInitialDir] = useState('')
  const [dragOver, setDragOver] = useState(false)

  useEffect(() => {
    if (!initialized) {
      refresh()
    }
  }, [initialized, refresh])

  const handleRemove = async (name: string): Promise<void> => {
    if (confirm(t('repos.removeConfirm', { name }))) {
      await removeRepo(name)
    }
  }

  const handleResolve = (name: string): void => {
    window.location.hash = `#/conflicts/${name}`
  }

  // Drag-drop handlers
  const handleDragOver = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer.types.includes('Files')) {
      setDragOver(true)
    }
  }, [])

  const handleDragLeave = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setDragOver(false)
  }, [])

  const handleDrop = useCallback(
    async (e: DragEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.stopPropagation()
      setDragOver(false)

      const files = e.dataTransfer.files
      for (let i = 0; i < files.length; i++) {
        const file = files[i]
        // On macOS/Electron, dragged folders have empty type and their path is in .path
        const path = (file as File & { path?: string }).path
        if (!path) continue

        try {
          const isGit = await window.api.isGitRepo(path)
          if (isGit) {
            // Git repo → add directly
            await addRepo(path)
          } else {
            // Non-git directory → open scan dialog with this dir pre-filled
            setScanInitialDir(path)
            setShowScan(true)
          }
        } catch {
          // Individual add errors handled by context
        }
      }
    },
    [addRepo]
  )

  return (
    <div
      className={`relative space-y-4 ${dragOver ? 'ring-2 ring-primary ring-offset-2 ring-offset-background rounded-lg' : ''}`}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Drag overlay */}
      {dragOver && (
        <div className="absolute inset-0 z-40 flex items-center justify-center rounded-lg bg-primary/5 border-2 border-dashed border-primary/40">
          <div className="text-center">
            <span className="text-4xl">📂</span>
            <p className="mt-2 text-sm font-medium text-primary">{t('repos.dropOverlay')}</p>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('repos.title')}</h2>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
            <span className={loading ? 'inline-block animate-spin' : ''}>
              {loading ? '⟳' : '🔄'}
            </span>{' '}
            {t('repos.refresh')}
          </Button>
          <Button variant="outline" size="sm" onClick={() => setShowScan(true)}>
            {t('repos.scan')}
          </Button>
          <Button size="sm" onClick={() => setShowAdd(true)}>
            {t('repos.addRepo')}
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {loading && repos.length === 0 && (
        <div className="py-8 text-center text-sm text-muted-foreground">{t('repos.loading')}</div>
      )}

      {!loading && repos.length === 0 && (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">{t('repos.emptyTitle')}</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('repos.emptyHint')}
          </p>
        </div>
      )}

      <div className="space-y-2">
        {repos.map((repo) => (
          <RepoRow
            key={repo.id}
            repo={repo}
            onSync={syncRepo}
            onRemove={handleRemove}
            onResolve={handleResolve}
            onSettings={setSettingsRepo}
          />
        ))}
      </div>

      <AddRepoDialog
        open={showAdd}
        onClose={() => setShowAdd(false)}
        onAdd={addRepo}
      />

      <ScanDialog
        open={showScan}
        onClose={() => { setShowScan(false); setScanInitialDir('') }}
        onScan={scan}
        onAdd={addRepo}
        scannedRepos={scannedRepos}
        loading={loading}
        initialDir={scanInitialDir}
      />

      <RepoSettingsDialog
        repoName={settingsRepo ?? ''}
        open={settingsRepo !== null}
        onClose={() => setSettingsRepo(null)}
      />
    </div>
  )
}
