import { useState, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCw } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { BranchMapping } from '@shared/types/engine'
import { Modal } from '@/components/ui/modal'
import { BranchMappingInput } from '@/components/BranchMappingInput'
import { engineApi } from '@/lib/api'

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

  const handleSelectDirectory = async (): Promise<void> => {
    try {
      const result = await engineApi.openDirectory()
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
      const result = await engineApi.scan(repoPath)
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

  const upstreamTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const handleUpstreamChange = (value: string): void => {
    setUpstream(value)
    if (path && value) {
      if (upstreamTimerRef.current) clearTimeout(upstreamTimerRef.current)
      upstreamTimerRef.current = setTimeout(() => loadBranches(path, value), 500)
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

  return (
    <Modal open={open} onClose={handleClose} maxWidth="max-w-lg" scrollable>
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
                  {loadingBranches ? t('common.loading') : (
                    <>
                      <RefreshCw size={12} className="mr-1" />
                      {t('common.refresh')}
                    </>
                  )}
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

          {/* Optional single branch mapping configuration */}
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
                    <BranchMappingInput
                      value={branchMapping}
                      onChange={setBranchMapping}
                      localBranches={localBranches}
                      remoteBranches={remoteBranches}
                    />
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
    </Modal>
  )
}
