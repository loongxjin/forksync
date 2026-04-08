import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { ScannedRepo } from '@/types/engine'

interface ScanDialogProps {
  open: boolean
  onClose: () => void
  onScan: (dir: string) => Promise<void>
  onAdd: (path: string, upstream?: string) => Promise<void>
  scannedRepos: ScannedRepo[]
  loading: boolean
}

export function ScanDialog({
  open,
  onClose,
  onScan,
  onAdd,
  scannedRepos,
  loading
}: ScanDialogProps): JSX.Element | null {
  const [dir, setDir] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [adding, setAdding] = useState(false)

  if (!open) return null

  const handleSelectDirectory = async (): Promise<void> => {
    try {
      const result = await window.api.openDirectory()
      if (!result.canceled && result.filePaths && result.filePaths.length > 0) {
        setDir(result.filePaths[0])
      }
    } catch (err) {
      console.error('Failed to open directory picker:', err)
    }
  }

  const handleScan = async (): Promise<void> => {
    if (!dir.trim()) return
    setSelected(new Set())
    await onScan(dir.trim())
  }

  const toggleSelect = (path: string): void => {
    const next = new Set(selected)
    if (next.has(path)) {
      next.delete(path)
    } else {
      next.add(path)
    }
    setSelected(next)
  }

  const handleAddSelected = async (): Promise<void> => {
    setAdding(true)
    try {
      for (const path of selected) {
        const repo = scannedRepos.find((r) => r.path === path)
        await onAdd(path, repo?.suggestedUpstream)
      }
      setSelected(new Set())
      onClose()
    } finally {
      setAdding(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-lg rounded-lg border border-border bg-card p-6 shadow-lg">
        <h3 className="text-lg font-semibold">Scan Directory</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Scan a directory for git repositories.
        </p>

        <div className="mt-4 space-y-2">
          <Label>Directory to Scan</Label>
          <div className="flex gap-2">
            <div 
              className="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground"
            >
              {dir || 'No directory selected'}
            </div>
            <Button 
              type="button" 
              variant="outline" 
              onClick={handleSelectDirectory}
            >
              选择目录
            </Button>
          </div>
          <Button 
            className="w-full" 
            onClick={handleScan} 
            disabled={!dir || loading}
          >
            {loading ? 'Scanning...' : 'Scan'}
          </Button>
        </div>

        {scannedRepos.length > 0 && (
          <div className="mt-4 max-h-64 space-y-2 overflow-y-auto">
            <Label className="text-xs">
              {scannedRepos.length} repos found — select to add:
            </Label>
            {scannedRepos.map((repo) => (
              <label
                key={repo.path}
                className="flex cursor-pointer items-start gap-2 rounded-md border border-border p-2 transition-colors hover:bg-accent/30"
              >
                <input
                  type="checkbox"
                  checked={selected.has(repo.path)}
                  onChange={() => toggleSelect(repo.path)}
                  className="mt-0.5"
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{repo.name}</span>
                    {repo.isFork && <Badge variant="info">fork</Badge>}
                  </div>
                  <p className="truncate text-xs text-muted-foreground">{repo.path}</p>
                  {repo.suggestedUpstream && (
                    <p className="truncate text-xs text-blue-400">
                      ↑ {repo.suggestedUpstream}
                    </p>
                  )}
                </div>
              </label>
            ))}
          </div>
        )}

        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          {selected.size > 0 && (
            <Button onClick={handleAddSelected} disabled={adding}>
              {adding ? `Adding ${selected.size}...` : `Add ${selected.size} repos`}
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
