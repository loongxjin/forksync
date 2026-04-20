import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { BranchMapping } from '@/types/engine'
import { ArrowRight } from 'lucide-react'

interface AddRepoDialogProps {
  open: boolean
  onClose: () => void
  onAdd: (path: string, upstream?: string, branchMapping?: BranchMapping) => Promise<void>
}

export function AddRepoDialog({ open, onClose, onAdd }: AddRepoDialogProps): JSX.Element | null {
  const { t } = useTranslation()
  const [path, setPath] = useState('')
  const [upstream, setUpstream] = useState('')
  const [adding, setAdding] = useState(false)
  
  const [localBranches, setLocalBranches] = useState<string[]>([])
  const [remoteBranches, setRemoteBranches] = useState<string[]>([])
  const [branchMapping, setBranchMapping] = useState<BranchMapping | undefined>(undefined)
  const [loadingBranches, setLoadingBranches] = useState(false)
  const [enableMapping, setEnableMapping] = useState(false)

  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    if (open) {
      setMounted(true)
    } else if (mounted) {
      const timer = setTimeout(() => setMounted(false), 200)
      return () => clearTimeout(timer)
    }
  }, [open, mounted])

  if (!mounted) return null

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
    <div className={`fixed inset-0 z-50 flex items-center justify-center transition-[visibility] duration-200 ${!open && 'invisible'}`}>
      <div
        className={`absolute inset-0 bg-black/50 backdrop-blur-sm transition-opacity duration-200 ${open ? 'opacity-100' : 'opacity-0'}`}
        onClick={handleClose}
      />
      <div
        className={`w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-xl border border-border bg-card p-6 shadow-2xl transition-all duration-200 ${
          open ? 'opacity-100 scale-100' : 'opacity-0 scale-95'
        }`}
      >
        <h3 className="text-lg font-semibold">{t('addRepo.title')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('addRepo.description')}
        </p>

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <div className="space-y-2">
            <Label>{t('addRepo.repoPath')}</Label>
            <div className="flex gap-2">
              <div 
                className="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground"
              >
                {path || t('addRepo.noDirectorySelected')}
              </div>
              <Button 
                type="button" 
                variant="outline" 
                onClick={handleSelectDirectory}
              >
                {t('common.selectDirectory')}
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="upstream">
                {t('addRepo.upstreamUrl')}
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
                  {loadingBranches ? t('common.loading') : `↻ ${t('common.refresh')}`}
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
              {t('addRepo.autoDetected')}
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
                    {t('addRepo.branchMapping')}
                  </Label>
                </div>
                <Badge variant="secondary">{t('common.optional')}</Badge>
              </div>
              
              {!enableMapping && (
                <p className="text-xs text-muted-foreground">
                  {t('addRepo.branchMappingHint')}
                </p>
              )}
              
              {enableMapping && (
                <>
                  <p className="text-xs text-muted-foreground">
                    {t('addRepo.branchMappingDesc')}
                  </p>
                  
                  {loadingBranches ? (
                    <div className="text-sm text-muted-foreground">{t('common.loading')}</div>
                  ) : (
                    <div className="flex items-center gap-3 p-3 rounded-md border border-border bg-background/50">
                      <div className="flex-1 space-y-1">
                        <Label className="text-xs">{t('addRepo.localBranch')}</Label>
                        {localBranches.length > 0 ? (
                          <select
                            value={branchMapping?.localBranch || ''}
                            onChange={(e) => updateBranchMapping('localBranch', e.target.value)}
                            className="w-full h-8 px-2 rounded-md border border-input bg-background text-sm"
                          >
                            <option value="">{t('common.select')}</option>
                            {localBranches.map(branch => (
                              <option key={branch} value={branch}>{branch}</option>
                            ))}
                          </select>
                        ) : (
                          <Input
                            placeholder={t('addRepo.localPlaceholder')}
                            value={branchMapping?.localBranch || ''}
                            onChange={(e) => updateBranchMapping('localBranch', e.target.value)}
                            className="h-8"
                          />
                        )}
                      </div>
                      
                      <ArrowRight size={16} className="text-muted-foreground" />
                      
                      <div className="flex-1 space-y-1">
                        <Label className="text-xs">{t('addRepo.remoteBranch')}</Label>
                        {remoteBranches.length > 0 ? (
                          <select
                            value={branchMapping?.remoteBranch || ''}
                            onChange={(e) => updateBranchMapping('remoteBranch', e.target.value)}
                            className="w-full h-8 px-2 rounded-md border border-input bg-background text-sm"
                          >
                            <option value="">{t('common.select')}</option>
                            {remoteBranches.map(branch => (
                              <option key={branch} value={branch}>{branch}</option>
                            ))}
                          </select>
                        ) : (
                          <Input
                            placeholder={t('addRepo.remotePlaceholder')}
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
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={!path || adding}>
              {adding ? t('addRepo.adding') : t('common.add')}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
