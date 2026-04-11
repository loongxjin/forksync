import { useState, useEffect } from 'react'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { ScannedRepo, BranchMapping } from '@/types/engine'

interface ScanDialogProps {
  open: boolean
  onClose: () => void
  onScan: (dir: string) => Promise<void>
  onAdd: (path: string, upstream?: string, branchMapping?: BranchMapping) => Promise<void>
  scannedRepos: ScannedRepo[]
  loading: boolean
  initialDir?: string
}

interface RepoBranchConfig {
  [repoPath: string]: {
    enabled: boolean
    mapping?: BranchMapping
  }
}

export function ScanDialog({
  open,
  onClose,
  onScan,
  onAdd,
  scannedRepos,
  loading,
  initialDir
}: ScanDialogProps): JSX.Element | null {
  const [dir, setDir] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [adding, setAdding] = useState(false)
  const [expandedRepo, setExpandedRepo] = useState<string | null>(null)
  const [repoBranchConfigs, setRepoBranchConfigs] = useState<RepoBranchConfig>({})

  // Auto-fill initialDir and trigger scan when provided (drag-drop flow)
  useEffect(() => {
    if (open && initialDir) {
      setDir(initialDir)
      setSelected(new Set())
      setRepoBranchConfigs({})
      setExpandedRepo(null)
      // Auto-trigger scan after a tick so dir state is set
      const timer = setTimeout(() => {
        onScan(initialDir)
      }, 50)
      return () => clearTimeout(timer)
    }
  }, [open, initialDir, onScan])

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
    setRepoBranchConfigs({})
    setExpandedRepo(null)
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

  const toggleExpand = (path: string): void => {
    setExpandedRepo(expandedRepo === path ? null : path)
  }

  const toggleMappingEnabled = (repoPath: string): void => {
    setRepoBranchConfigs(prev => ({
      ...prev,
      [repoPath]: {
        ...prev[repoPath],
        enabled: !(prev[repoPath]?.enabled ?? false)
      }
    }))
  }

  const updateRepoBranchMapping = (repoPath: string, field: keyof BranchMapping, value: string): void => {
    setRepoBranchConfigs(prev => ({
      ...prev,
      [repoPath]: {
        ...prev[repoPath],
        mapping: {
          ...prev[repoPath]?.mapping,
          localBranch: field === 'localBranch' ? value : (prev[repoPath]?.mapping?.localBranch || ''),
          remoteBranch: field === 'remoteBranch' ? value : (prev[repoPath]?.mapping?.remoteBranch || '')
        }
      }
    }))
  }

  const handleAddSelected = async (): Promise<void> => {
    setAdding(true)
    try {
      for (const path of selected) {
        const repo = scannedRepos.find((r) => r.path === path)
        const config = repoBranchConfigs[path]
        const branchMapping = config?.enabled && config?.mapping?.localBranch && config?.mapping?.remoteBranch
          ? config.mapping
          : undefined
        await onAdd(path, repo?.suggestedUpstream, branchMapping)
      }
      setSelected(new Set())
      setRepoBranchConfigs({})
      onClose()
    } finally {
      setAdding(false)
    }
  }

  const handleClose = (): void => {
    setDir('')
    setSelected(new Set())
    setRepoBranchConfigs({})
    setExpandedRepo(null)
    onClose()
  }

  const selectAll = (): void => {
    if (selected.size === scannedRepos.length) {
      setSelected(new Set())
    } else {
      setSelected(new Set(scannedRepos.map(r => r.path)))
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-lg border border-border bg-card p-6 shadow-lg">
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
          <div className="mt-4 space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs">
                {scannedRepos.length} repos found
              </Label>
              <Button 
                variant="ghost" 
                size="sm" 
                onClick={selectAll}
                className="h-6 text-xs"
              >
                {selected.size === scannedRepos.length ? 'Deselect All' : 'Select All'}
              </Button>
            </div>
            <div className="max-h-80 space-y-2 overflow-y-auto">
              {scannedRepos.map((repo) => {
                const isExpanded = expandedRepo === repo.path
                const config = repoBranchConfigs[repo.path] || { enabled: false }
                const hasMapping = config.enabled && config.mapping?.localBranch && config.mapping?.remoteBranch
                
                return (
                  <div 
                    key={repo.path} 
                    className={`rounded-md border transition-colors ${
                      selected.has(repo.path) ? 'border-primary/50 bg-accent/30' : 'border-border'
                    }`}
                  >
                    <label className="flex cursor-pointer items-start gap-2 p-2">
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
                          {hasMapping && <Badge variant="secondary">mapped</Badge>}
                        </div>
                        <p className="truncate text-xs text-muted-foreground">{repo.path}</p>
                        {repo.suggestedUpstream && (
                          <p className="truncate text-xs text-blue-400">
                            ↑ {repo.suggestedUpstream}
                          </p>
                        )}
                      </div>
                      {selected.has(repo.path) && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            toggleExpand(repo.path)
                          }}
                          className="h-6 text-xs"
                        >
                          {isExpanded ? 'Hide' : 'Map'}
                        </Button>
                      )}
                    </label>
                    
                    {/* 单个分支映射配置面板 */}
                    {isExpanded && selected.has(repo.path) && (
                      <div className="border-t border-border bg-background/50 p-3 space-y-3">
                        <div className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            id={`enable-mapping-${repo.path}`}
                            checked={config.enabled}
                            onChange={() => toggleMappingEnabled(repo.path)}
                            className="rounded border-input"
                          />
                          <Label htmlFor={`enable-mapping-${repo.path}`} className="text-xs cursor-pointer">
                            Custom Branch Mapping
                          </Label>
                        </div>
                        
                        {!config.enabled && (
                          <p className="text-xs text-muted-foreground">
                            Default: sync branches with the same name
                          </p>
                        )}
                        
                        {config.enabled && (
                          <div className="flex items-center gap-2 p-2 rounded bg-background border border-border">
                            <div className="flex-1">
                              {repo.localBranches && repo.localBranches.length > 0 ? (
                                <select
                                  value={config.mapping?.localBranch || ''}
                                  onChange={(e) => updateRepoBranchMapping(repo.path, 'localBranch', e.target.value)}
                                  className="w-full h-7 px-2 rounded border border-input bg-background text-xs"
                                >
                                  <option value="">Local branch</option>
                                  {repo.localBranches.map(branch => (
                                    <option key={branch} value={branch}>{branch}</option>
                                  ))}
                                </select>
                              ) : (
                                <input
                                  type="text"
                                  placeholder="local branch"
                                  value={config.mapping?.localBranch || ''}
                                  onChange={(e) => updateRepoBranchMapping(repo.path, 'localBranch', e.target.value)}
                                  className="w-full h-7 px-2 rounded border border-input bg-background text-xs"
                                />
                              )}
                            </div>
                            <span className="text-muted-foreground text-xs">→</span>
                            <div className="flex-1">
                              {repo.remoteBranches && repo.remoteBranches.length > 0 ? (
                                <select
                                  value={config.mapping?.remoteBranch || ''}
                                  onChange={(e) => updateRepoBranchMapping(repo.path, 'remoteBranch', e.target.value)}
                                  className="w-full h-7 px-2 rounded border border-input bg-background text-xs"
                                >
                                  <option value="">Remote branch</option>
                                  {repo.remoteBranches.map(branch => (
                                    <option key={branch} value={branch}>{branch}</option>
                                  ))}
                                </select>
                              ) : (
                                <input
                                  type="text"
                                  placeholder="remote branch"
                                  value={config.mapping?.remoteBranch || ''}
                                  onChange={(e) => updateRepoBranchMapping(repo.path, 'remoteBranch', e.target.value)}
                                  className="w-full h-7 px-2 rounded border border-input bg-background text-xs"
                                />
                              )}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        )}

        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={handleClose}>
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
