import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import type { PostSyncCommand, BranchMapping } from '@shared/types/engine'
import { engineApi } from '@/lib/api'
import { useRepos } from '@/contexts/RepoContext'
import { Trash2, X } from 'lucide-react'
import { Modal } from '@/components/ui/modal'
import { BranchMappingInput } from '@/components/BranchMappingInput'
import { Separator } from '@/components/ui/separator'
import { useToastContext } from '@/contexts/ToastContext'

interface RepoSettingsDialogProps {
  repoName: string
  open: boolean
  onClose: () => void
  /** 'all' (default): branch mapping + post-sync. 'postSync': post-sync only. */
  section?: 'all' | 'postSync'
}

function autoName(cmd: string): string {
  const parts = cmd.trim().split(/\s+/)
  return parts[0] || cmd.trim()
}

export function RepoSettingsDialog({ repoName, open, onClose, section = 'all' }: RepoSettingsDialogProps): JSX.Element | null {
  const { t } = useTranslation()
  const { repos, updateRepo } = useRepos()
  const { showToast } = useToastContext()

  const [commands, setCommands] = useState<PostSyncCommand[]>([])
  const [loading, setLoading] = useState(false)
  const [showAddForm, setShowAddForm] = useState(false)
  const [newCmd, setNewCmd] = useState('')
  const [saving, setSaving] = useState(false)

  const repo = repos.find((r) => r.name === repoName)
  const [mapping, setMapping] = useState<BranchMapping | undefined>(repo?.branchMapping)
  const [localBranches, setLocalBranches] = useState<string[]>([])
  const [remoteBranches, setRemoteBranches] = useState<string[]>([])
  const [savingMapping, setSavingMapping] = useState(false)

  const loadCommands = useCallback(async () => {
    setLoading(true)
    try {
      const res = await engineApi.postSyncList(repoName)
      if (res.success) {
        setCommands(res.data.commands ?? [])
      }
    } catch {
      // silent
    } finally {
      setLoading(false)
    }
  }, [repoName])

  useEffect(() => {
    if (open) {
      loadCommands()
      setShowAddForm(false)
      setNewCmd('')
      const r = repos.find((r) => r.name === repoName)
      setMapping(r?.branchMapping)
      engineApi.repoBranches(repoName).then((res) => {
        if (res.success) {
          setLocalBranches(res.data.localBranches ?? [])
          setRemoteBranches(res.data.remoteBranches ?? [])
        }
      })
    }
  }, [open, loadCommands, repoName, repos])

  const handleAdd = async (): Promise<void> => {
    if (!newCmd.trim()) return
    setSaving(true)
    try {
      const res = await engineApi.postSyncAdd(repoName, autoName(newCmd), newCmd.trim())
      if (res.success) {
        setCommands(res.data.commands ?? [])
        setNewCmd('')
        setShowAddForm(false)
      }
    } catch {
      // silent
    } finally {
      setSaving(false)
    }
  }

  const handleRemove = async (cmdId: string): Promise<void> => {
    setSaving(true)
    try {
      const res = await engineApi.postSyncRemove(repoName, cmdId)
      if (res.success) {
        setCommands(res.data.commands ?? [])
      }
    } catch {
      // silent
    } finally {
      setSaving(false)
    }
  }

  const handleSaveMapping = async (): Promise<void> => {
    if (!mapping?.localBranch || !mapping?.remoteBranch) return
    setSavingMapping(true)
    try {
      const res = await engineApi.setBranchMapping(repoName, mapping.localBranch, mapping.remoteBranch)
      if (res.success && repo) {
        updateRepo({ ...repo, branchMapping: mapping })
      }
      showToast?.(t('repos.branchMappingSaved'), 'success')
    } catch {
      // silent
    } finally {
      setSavingMapping(false)
    }
  }

  const mappingChanged =
    (mapping?.localBranch ?? '') !== (repo?.branchMapping?.localBranch ?? '') ||
    (mapping?.remoteBranch ?? '') !== (repo?.branchMapping?.remoteBranch ?? '')

  return (
    <Modal open={open} onClose={onClose} maxWidth="max-w-lg">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-semibold">{section === 'postSync' ? t('postSync.title') : t('repos.repoSettings')}</h2>
        <button
          onClick={onClose}
          className="rounded px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <X size={16} />
        </button>
      </div>

      {/* Branch Mapping — only in full settings */}
      {section !== 'postSync' && (
      <div className="mb-6">
        <h3 className="mb-2 text-sm font-medium">{t('repos.branchMapping')}</h3>
        <p className="mb-3 text-xs text-muted-foreground">{t('repos.branchMappingHint')}</p>
        <BranchMappingInput
          value={mapping}
          onChange={setMapping}
          localBranches={localBranches}
          remoteBranches={remoteBranches}
        />
        <button
          onClick={handleSaveMapping}
          disabled={!mappingChanged || savingMapping}
          className="mt-3 h-8 rounded bg-primary px-3 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          {t('common.save')}
        </button>
      </div>
      )}

      {section !== 'postSync' && <Separator />}

      {/* Post-Sync Commands */}
      <div className="mt-4">
        <h3 className="mb-2 text-sm font-medium">
          {t('postSync.title')} ({commands.length})
        </h3>
        <p className="mb-3 text-xs text-muted-foreground">{t('postSync.description')}</p>

        {loading ? (
          <p className="py-4 text-center text-sm text-muted-foreground">{t('common.loading')}</p>
        ) : commands.length === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">{t('postSync.empty')}</p>
        ) : (
          <div className="mb-4 space-y-2">
            {commands.map((cmd) => (
              <div
                key={cmd.id}
                className="flex items-center justify-between rounded border border-border bg-background px-3 py-2"
              >
                <code className="min-w-0 flex-1 truncate text-sm">{cmd.cmd}</code>
                <button
                  onClick={() => handleRemove(cmd.id)}
                  disabled={saving}
                  className="ml-2 shrink-0 rounded px-1.5 py-0.5 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-500 disabled:opacity-50"
                >
                  <Trash2 size={12} className="text-muted-foreground hover:text-error" />
                </button>
              </div>
            ))}
          </div>
        )}

        {showAddForm ? (
          <div className="flex items-center gap-2">
            <input
              type="text"
              placeholder={t('postSync.commandPlaceholder')}
              value={newCmd}
              onChange={(e) => setNewCmd(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleAdd()
                if (e.key === 'Escape') {
                  setShowAddForm(false)
                  setNewCmd('')
                }
              }}
              autoFocus
              className="flex-1 rounded border border-border bg-card px-3 py-1.5 text-sm font-mono placeholder:text-muted-foreground focus:border-primary focus:outline-none"
            />
            <button
              onClick={handleAdd}
              disabled={saving || !newCmd.trim()}
              className="shrink-0 rounded bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {t('common.add')}
            </button>
            <button
              onClick={() => {
                setShowAddForm(false)
                setNewCmd('')
              }}
              className="shrink-0 rounded px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent"
            >
              {t('common.cancel')}
            </button>
          </div>
        ) : (
          <button
            onClick={() => setShowAddForm(true)}
            className="w-full rounded border border-dashed border-border px-3 py-2 text-sm text-muted-foreground hover:border-primary hover:text-foreground"
          >
            + {t('postSync.addCommand')}
          </button>
        )}
      </div>
    </Modal>
  )
}
