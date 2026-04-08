/**
 * IDEConfig — Settings page IDE configuration component
 */

import { useState, useCallback } from 'react'
import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'

export function IDEConfig(): JSX.Element {
  const {
    ideConfig,
    ideLoading,
    setDefaultIDE,
    addCustomIDE,
    removeCustomIDE,
    refreshIDEConfig,
    getInstalledIDEs
  } = useSettings()

  const [detecting, setDetecting] = useState(false)
  const [showAddForm, setShowAddForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [newCli, setNewCli] = useState('')

  const installedIDEs = getInstalledIDEs()

  const handleRedetect = useCallback(async () => {
    setDetecting(true)
    await refreshIDEConfig()
    setDetecting(false)
  }, [refreshIDEConfig])

  const handleAddCustom = useCallback(async () => {
    if (!newName.trim() || !newCli.trim()) return
    await addCustomIDE(newName.trim(), newCli.trim())
    setNewName('')
    setNewCli('')
    setShowAddForm(false)
  }, [newName, newCli, addCustomIDE])

  const handleRemoveCustom = useCallback(
    async (ideId: string) => {
      await removeCustomIDE(ideId)
    },
    [removeCustomIDE]
  )

  if (ideLoading) {
    return (
      <div className="space-y-2">
        <Label>IDE 配置</Label>
        <p className="text-xs text-muted-foreground">加载中...</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Default IDE selector */}
      <div className="space-y-2">
        <Label>默认 IDE</Label>
        <select
          value={ideConfig?.defaultIDE ?? ''}
          onChange={(e) => setDefaultIDE(e.target.value || null)}
          className="rounded-md border border-border bg-card px-3 py-1.5 text-sm text-foreground"
        >
          <option value="">未设置</option>
          {installedIDEs.map((ide) => (
            <option key={ide.id} value={ide.id}>
              {ide.name}
            </option>
          ))}
        </select>
      </div>

      {/* Detected IDEs */}
      <div className="space-y-2">
        <Label>已检测到的 IDE</Label>
        <div className="space-y-1">
          {ideConfig?.detectedIDEs
            .filter((ide) => !ide.isCustom)
            .map((ide) => (
              <div key={ide.id} className="flex items-center gap-2 text-xs">
                <span>{ide.installed ? '✅' : '❌'}</span>
                <span className="text-foreground">{ide.name}</span>
                {ide.installed && (
                  <span className="text-muted-foreground">(CLI: {ide.cliCommand})</span>
                )}
                {!ide.installed && (
                  <span className="text-muted-foreground">未检测到</span>
                )}
              </div>
            ))}
        </div>
      </div>

      {/* Custom IDEs */}
      <div className="space-y-2">
        <Label>自定义 IDE</Label>
        {ideConfig?.customIDEs && ideConfig.customIDEs.length > 0 ? (
          <div className="space-y-1">
            {ideConfig.customIDEs.map((custom) => {
              const detected = ideConfig.detectedIDEs.find((i) => i.id === custom.id)
              return (
                <div key={custom.id} className="flex items-center gap-2 text-xs">
                  <span>{detected?.installed ? '✅' : '❌'}</span>
                  <span className="text-foreground">{custom.name}</span>
                  <span className="text-muted-foreground">(CLI: {custom.cliCommand})</span>
                  <button
                    onClick={() => handleRemoveCustom(custom.id)}
                    className="text-red-400 hover:text-red-500"
                  >
                    ✕
                  </button>
                </div>
              )
            })}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">暂无自定义 IDE</p>
        )}

        {showAddForm ? (
          <div className="space-y-2 rounded-md border border-border p-3">
            <input
              type="text"
              placeholder="IDE 名称 (如: Windsurf)"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="w-full rounded border border-border bg-card px-2 py-1 text-xs text-foreground"
            />
            <input
              type="text"
              placeholder="CLI 命令 (如: windsurf)"
              value={newCli}
              onChange={(e) => setNewCli(e.target.value)}
              className="w-full rounded border border-border bg-card px-2 py-1 text-xs text-foreground"
            />
            <div className="flex gap-2">
              <button
                onClick={handleAddCustom}
                className="rounded bg-primary px-3 py-1 text-xs text-primary-foreground hover:bg-primary/90"
              >
                添加
              </button>
              <button
                onClick={() => setShowAddForm(false)}
                className="rounded border border-border px-3 py-1 text-xs text-foreground hover:bg-accent"
              >
                取消
              </button>
            </div>
          </div>
        ) : (
          <button
            onClick={() => setShowAddForm(true)}
            className="text-xs text-primary hover:underline"
          >
            + 添加自定义 IDE
          </button>
        )}
      </div>

      {/* Redetect button */}
      <button
        onClick={handleRedetect}
        disabled={detecting}
        className="rounded border border-border px-3 py-1.5 text-xs text-foreground hover:bg-accent disabled:opacity-50"
      >
        {detecting ? '检测中...' : '🔍 重新检测'}
      </button>
    </div>
  )
}
