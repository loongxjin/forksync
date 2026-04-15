import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import type { PostSyncCommand } from '@/types/engine'
import { engineApi } from '@/lib/api'

interface RepoSettingsDialogProps {
  repoName: string
  open: boolean
  onClose: () => void
}

function autoName(cmd: string): string {
  const parts = cmd.trim().split(/\s+/)
  return parts[0] || cmd.trim()
}

export function RepoSettingsDialog({ repoName, open, onClose }: RepoSettingsDialogProps): JSX.Element | null {
  const { t } = useTranslation()
  const [commands, setCommands] = useState<PostSyncCommand[]>([])
  const [loading, setLoading] = useState(false)
  const [showAddForm, setShowAddForm] = useState(false)
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
      setNewCmd('')
    }
  }, [open, loadCommands])

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
                <code className="min-w-0 flex-1 truncate text-sm">{cmd.cmd}</code>
                <button
                  onClick={() => handleRemove(cmd.id)}
                  disabled={saving}
                  className="ml-2 shrink-0 rounded px-1.5 py-0.5 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-500 disabled:opacity-50"
                >
                  🗑
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Add command form */}
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
    </div>
  )
}
