import { useEffect, useState } from 'react'
import { useRepos } from '@/contexts/RepoContext'
import { RepoRow } from '@/components/RepoRow'
import { AddRepoDialog } from '@/components/AddRepoDialog'
import { ScanDialog } from '@/components/ScanDialog'
import { Button } from '@/components/ui/button'

export function Repos(): JSX.Element {
  const { repos, scannedRepos, loading, error, refresh, syncRepo, scan, addRepo, removeRepo } =
    useRepos()
  const [showAdd, setShowAdd] = useState(false)
  const [showScan, setShowScan] = useState(false)

  useEffect(() => {
    refresh()
  }, [refresh])

  const handleRemove = async (name: string): Promise<void> => {
    if (confirm(`Remove "${name}" from ForkSync? The local repo won't be deleted.`)) {
      await removeRepo(name)
    }
  }

  const handleResolve = (name: string): void => {
    // Navigate to conflicts page — will be implemented in Task 10
    window.location.hash = `#/conflicts/${name}`
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Repositories</h2>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setShowScan(true)}>
            📂 Scan
          </Button>
          <Button size="sm" onClick={() => setShowAdd(true)}>
            + Add Repo
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {loading && repos.length === 0 && (
        <div className="py-8 text-center text-sm text-muted-foreground">Loading...</div>
      )}

      {!loading && repos.length === 0 && (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">No repositories managed yet.</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Click <strong>+ Add Repo</strong> or <strong>📂 Scan</strong> to get started.
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
        onClose={() => setShowScan(false)}
        onScan={scan}
        onAdd={addRepo}
        scannedRepos={scannedRepos}
        loading={loading}
      />
    </div>
  )
}
