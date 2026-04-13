/**
 * IDEConfig — Settings page IDE configuration component
 */

import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'

export function IDEConfig(): JSX.Element {
  const { t } = useTranslation()
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
        <Label>{t('ide.config')}</Label>
        <p className="text-xs text-muted-foreground">{t('common.loading')}</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Default IDE selector */}
      <div className="space-y-2">
        <Label>{t('ide.defaultIde')}</Label>
        <select
          value={ideConfig?.defaultIDE ?? ''}
          onChange={(e) => setDefaultIDE(e.target.value || null)}
          className="rounded-md border border-border bg-card px-3 py-1.5 text-sm text-foreground"
        >
          <option value="">{t('ide.notSet')}</option>
          {installedIDEs.map((ide) => (
            <option key={ide.id} value={ide.id}>
              {ide.name}
            </option>
          ))}
        </select>
      </div>

      {/* Detected IDEs */}
      <div className="space-y-2">
        <Label>{t('ide.detectedIdes')}</Label>
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
                  <span className="text-muted-foreground">{t('ide.notDetected')}</span>
                )}
              </div>
            ))}
        </div>
      </div>

      {/* Custom IDEs */}
      <div className="space-y-2">
        <Label>{t('ide.customIde')}</Label>
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
          <p className="text-xs text-muted-foreground">{t('ide.noCustomIde')}</p>
        )}

        {showAddForm ? (
          <div className="space-y-2 rounded-md border border-border p-3">
            <input
              type="text"
              placeholder={t('ide.namePlaceholder')}
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="w-full rounded border border-border bg-card px-2 py-1 text-xs text-foreground"
            />
            <input
              type="text"
              placeholder={t('ide.commandPlaceholder')}
              value={newCli}
              onChange={(e) => setNewCli(e.target.value)}
              className="w-full rounded border border-border bg-card px-2 py-1 text-xs text-foreground"
            />
            <div className="flex gap-2">
              <button
                onClick={handleAddCustom}
                className="rounded bg-primary px-3 py-1 text-xs text-primary-foreground hover:bg-primary/90"
              >
                {t('common.add')}
              </button>
              <button
                onClick={() => setShowAddForm(false)}
                className="rounded border border-border px-3 py-1 text-xs text-foreground hover:bg-accent"
              >
                {t('common.cancel')}
              </button>
            </div>
          </div>
        ) : (
          <button
            onClick={() => setShowAddForm(true)}
            className="text-xs text-primary hover:underline"
          >
            {t('ide.addCustom')}
          </button>
        )}
      </div>

      {/* Redetect button */}
      <button
        onClick={handleRedetect}
        disabled={detecting}
        className="rounded border border-border px-3 py-1.5 text-xs text-foreground hover:bg-accent disabled:opacity-50"
      >
        {detecting ? t('ide.detecting') : t('ide.redetect')}
      </button>
    </div>
  )
}
