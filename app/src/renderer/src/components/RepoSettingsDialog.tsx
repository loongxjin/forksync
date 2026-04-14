import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import type { PostSyncCommand } from '@/types/engine'
import { engineApi } from '@/lib/api'

interface RepoSettingsDialogProps {
  repoName: string
  open: boolean
  onClose: () => void
}

export function RepoSettingsDialog({ repoName, open, onClose }: RepoSettingsDialogProps): JSX.Element | null {
  const { t } = useTranslation()
  const [commands, setCommands] = useState<PostSyncCommand[]>([])
  const [loading, setLoading] = useState(false)
  const [showAddForm, setShowAddForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [newCmd, setNewCmd] = useState('')
  const [saving, setSaving] = useState(false)

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
      setNewName('')
      setNewCmd('')
    }
  }, [open, loadCommands])

  const handleAdd = async (): Promise<void> => {
    if (!newName.trim() || !newCmd.trim()) return
    setSaving(true)
    try {
      const res = await engineApi.postSyncAdd(repoName, newName.trim(), newCmd.trim())
      if (res.success) {
        setCommands(res.data.commands ?? [])
        setNewName('')
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

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />

      {/* Dialog */}
      <div className="relative z-10 w-full max-w-lg rounded-lg border border-border bg-card p-6 shadow-lg">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">{t('postSync.title')}</h2>
          <button
            onClick={onClose}
            className="rounded px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            ✕
          </button>
        </div>

        <p className="mb-4 text-sm text-muted-foreground">{t('postSync.description')}</p>

        {/* Command list */}
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
                <div className="min-w-0 flex-1">
                  <span className="text-sm font-medium">{cmd.name}</span>
                  <code className="ml-2 text-xs text-muted-foreground">{cmd.cmd}</code>
                </div>
                <button
                  onClick={() => handleRemove(cmd.id)}
                  disabled={saving}
                  className="ml-2 rounded px-1.5 py-0.5 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-500 disabled:opacity-50"
                >
                  🗑
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Add command form */}
        {showAddForm ? (
          <div className="space-y-2 rounded border border-border bg-background p-3">
            <input
              type="text"
              placeholder={t('postSync.commandNamePlaceholder')}
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="w-full rounded border border-border bg-card px-3 py-1.5 text-sm placeholder:text-muted-foreground focus:border-primary focus:outline-none"
            />
            <input
              type="text"
              placeholder={t('postSync.commandPlaceholder')}
              value={newCmd}
              onChange={(e) => setNewCmd(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleAdd()
              }}
              className="w-full rounded border border-border bg-card px-3 py-1.5 text-sm font-mono placeholder:text-muted-foreground focus:border-primary focus:outline-none"
            />
            <div className="flex justify-end gap-2">
              <button
                onClick={() => {
                  setShowAddForm(false)
                  setNewName('')
                  setNewCmd('')
                }}
                className="rounded px-3 py-1 text-xs text-muted-foreground hover:bg-accent"
              >
                {t('common.cancel')}
              </button>
              <button
                onClick={handleAdd}
                disabled={saving || !newName.trim() || !newCmd.trim()}
                className="rounded bg-primary px-3 py-1 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {saving ? t('common.saving') : t('common.add')}
              </button>
            </div>
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
    </div>
  )
}
