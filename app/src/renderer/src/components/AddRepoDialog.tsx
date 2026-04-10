import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { BranchMapping } from '@/types/engine'

interface AddRepoDialogProps {
  open: boolean
  onClose: () => void
  onAdd: (path: string, upstream?: string, branchMapping?: BranchMapping) => Promise<void>
}

export function AddRepoDialog({ open, onClose, onAdd }: AddRepoDialogProps): JSX.Element | null {
  const [path, setPath] = useState('')
  const [upstream, setUpstream] = useState('')
  const [adding, setAdding] = useState(false)
  
  const [localBranches, setLocalBranches] = useState<string[]>([])
  const [remoteBranches, setRemoteBranches] = useState<string[]>([])
  const [branchMapping, setBranchMapping] = useState<BranchMapping | undefined>(undefined)
  const [loadingBranches, setLoadingBranches] = useState(false)
  const [enableMapping, setEnableMapping] = useState(false)

  if (!open) return null

  const handleSelectDirectory = async (): Promise<void> => {
    try {
      const result = await window.api.openDirectory()
      if (!result.canceled && result.filePaths && result.filePaths.length > 0) {
        const selectedPath = result.filePaths[0]
        setPath(selectedPath)
        await loadBranches(selectedPath, upstream)
      }
    } catch (err) {
      console.error('Failed to open directory picker:', err)
    }
  }

  const loadBranches = async (repoPath: string, upstreamUrl?: string): Promise<void> => {
    setLoadingBranches(true)
    try {
      const result = await window.api.scan(repoPath)
      if (result.success && result.data.repos && result.data.repos.length > 0) {
        const scannedRepo = result.data.repos[0]
        setLocalBranches(scannedRepo.localBranches || [])
        setRemoteBranches(scannedRepo.remoteBranches || [])
        if (scannedRepo.suggestedUpstream && !upstreamUrl) {
          setUpstream(scannedRepo.suggestedUpstream)
        }
      }
    } catch (err) {
      console.error('Failed to load branches:', err)
    } finally {
      setLoadingBranches(false)
    }
  }

  const handleUpstreamChange = (value: string): void => {
    setUpstream(value)
    if (path && value) {
      setTimeout(() => loadBranches(path, value), 500)
    }
  }

  const handleSubmit = async (e: React.FormEvent): Promise<void> => {
    e.preventDefault()
    if (!path.trim()) return
    setAdding(true)
    try {
      const finalMapping = enableMapping && branchMapping?.localBranch && branchMapping?.remoteBranch
        ? branchMapping
        : undefined
      await onAdd(path.trim(), upstream.trim() || undefined, finalMapping)
      resetForm()
      onClose()
    } finally {
      setAdding(false)
    }
  }

  const resetForm = (): void => {
    setPath('')
    setUpstream('')
    setBranchMapping(undefined)
    setEnableMapping(false)
    setLocalBranches([])
    setRemoteBranches([])
  }

  const handleClose = (): void => {
    resetForm()
    onClose()
  }

  const updateBranchMapping = (field: keyof BranchMapping, value: string): void => {
    setBranchMapping(prev => ({
      ...prev,
      localBranch: field === 'localBranch' ? value : (prev?.localBranch || ''),
      remoteBranch: field === 'remoteBranch' ? value : (prev?.remoteBranch || '')
    }))
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-lg border border-border bg-card p-6 shadow-lg">
        <h3 className="text-lg font-semibold">Add Repository</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Enter the local path to a git repository.
        </p>

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <div className="space-y-2">
            <Label>Repository Path</Label>
            <div className="flex gap-2">
              <div 
                className="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground"
              >
                {path || 'No directory selected'}
              </div>
              <Button 
                type="button" 
                variant="outline" 
                onClick={handleSelectDirectory}
              >
                选择目录
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="upstream">
                Upstream URL <span className="text-muted-foreground">(optional)</span>
              </Label>
              {path && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => loadBranches(path, upstream)}
                  disabled={loadingBranches}
                  className="h-6 text-xs"
                >
                  {loadingBranches ? 'Loading...' : '↻ Refresh branches'}
                </Button>
              )}
            </div>
            <Input
              id="upstream"
              placeholder="https://github.com/original/repo.git"
              value={upstream}
              onChange={(e) => handleUpstreamChange(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Auto-detected for GitHub forks.
            </p>
          </div>

          {/* 单个可选的分支映射配置 */}
          {path && (
            <div className="space-y-3 border-t border-border pt-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="enable-mapping"
                    checked={enableMapping}
                    onChange={(e) => {
                      setEnableMapping(e.target.checked)
                      if (!e.target.checked) {
                        setBranchMapping(undefined)
                      }
                    }}
                    className="rounded border-input"
                  />
                  <Label htmlFor="enable-mapping" className="cursor-pointer">
                    Custom Branch Mapping
                  </Label>
                </div>
                <Badge variant="secondary">Optional</Badge>
              </div>
              
              {!enableMapping && (
                <p className="text-xs text-muted-foreground">
                  Default: sync branches with the same name (e.g., main → origin/main)
                </p>
              )}
              
              {enableMapping && (
                <>
                  <p className="text-xs text-muted-foreground">
                    Map a local branch to a different remote branch name.
                  </p>
                  
                  {loadingBranches ? (
                    <div className="text-sm text-muted-foreground">Loading branches...</div>
                  ) : (
                    <div className="flex items-center gap-3 p-3 rounded-md border border-border bg-background/50">
                      <div className="flex-1 space-y-1">
                        <Label className="text-xs">Local Branch</Label>
                        {localBranches.length > 0 ? (
                          <select
                            value={branchMapping?.localBranch || ''}
                            onChange={(e) => updateBranchMapping('localBranch', e.target.value)}
                            className="w-full h-8 px-2 rounded-md border border-input bg-background text-sm"
                          >
                            <option value="">Select...</option>
                            {localBranches.map(branch => (
                              <option key={branch} value={branch}>{branch}</option>
                            ))}
                          </select>
                        ) : (
                          <Input
                            placeholder="e.g., main"
                            value={branchMapping?.localBranch || ''}
                            onChange={(e) => updateBranchMapping('localBranch', e.target.value)}
                            className="h-8"
                          />
                        )}
                      </div>
                      
                      <div className="text-muted-foreground pt-5">→</div>
                      
                      <div className="flex-1 space-y-1">
                        <Label className="text-xs">Remote Branch</Label>
                        {remoteBranches.length > 0 ? (
                          <select
                            value={branchMapping?.remoteBranch || ''}
                            onChange={(e) => updateBranchMapping('remoteBranch', e.target.value)}
                            className="w-full h-8 px-2 rounded-md border border-input bg-background text-sm"
                          >
                            <option value="">Select...</option>
                            {remoteBranches.map(branch => (
                              <option key={branch} value={branch}>{branch}</option>
                            ))}
                          </select>
                        ) : (
                          <Input
                            placeholder="e.g., master"
                            value={branchMapping?.remoteBranch || ''}
                            onChange={(e) => updateBranchMapping('remoteBranch', e.target.value)}
                            className="h-8"
                          />
                        )}
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="ghost" onClick={handleClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!path || adding}>
              {adding ? 'Adding...' : 'Add'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
