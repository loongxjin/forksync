import { useState, useEffect, useRef } from 'react'
import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { IDEConfig } from '@/components/IDEConfig'

/** A simple toggle switch component */
function Toggle({
  checked,
  onChange,
  disabled = false,
  label
}: {
  checked: boolean
  onChange: (val: boolean) => void
  disabled?: boolean
  label: string
}): JSX.Element {
  return (
    <div className="flex items-center justify-between">
      <Label className="cursor-pointer">{label}</Label>
      <button
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 ${
          checked ? 'bg-primary' : 'bg-input'
        }`}
      >
        <span
          className={`pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform ${
            checked ? 'translate-x-4' : 'translate-x-0'
          }`}
        />
      </button>
    </div>
  )
}

export function GeneralSettings(): JSX.Element {
  const { theme, setTheme, engineConfig, configLoading, updateConfig } = useSettings()

  // Local state for sync interval (debounced save)
  const [syncInterval, setSyncInterval] = useState('')
  const [saving, setSaving] = useState(false)
  const isEditingRef = useRef(false)
  const prevConfigIntervalRef = useRef('')

  // Sync local state from engine config
  useEffect(() => {
    const configInterval = engineConfig?.Sync?.DefaultInterval ?? ''
    if (configInterval !== '' && !isEditingRef.current && configInterval !== prevConfigIntervalRef.current) {
      setSyncInterval(configInterval)
      prevConfigIntervalRef.current = configInterval
    }
  }, [engineConfig])

  // Debounced save for sync interval
  useEffect(() => {
    if (!syncInterval || !engineConfig) return
    if (syncInterval === engineConfig.Sync?.DefaultInterval) return

    isEditingRef.current = true
    const timer = setTimeout(async () => {
      setSaving(true)
      await updateConfig('sync.default_interval', syncInterval)
      setSaving(false)
      isEditingRef.current = false
      prevConfigIntervalRef.current = syncInterval
    }, 1500)

    return () => clearTimeout(timer)
  }, [syncInterval, engineConfig, updateConfig])

  const handleSyncOnStartup = async (val: boolean): Promise<void> => {
    await updateConfig('sync.sync_on_startup', String(val))
  }

  const handleAutoLaunch = async (val: boolean): Promise<void> => {
    await updateConfig('sync.auto_launch', String(val))
    // Also update OS login item
    await window.api.setAutoLaunch(val)
  }

  const isLoading = configLoading || !engineConfig

  return (
    <div className="space-y-6">
      {/* Theme */}
      <div className="space-y-2">
        <Label>Theme</Label>
        <div className="flex gap-2">
          {(['dark', 'light', 'system'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTheme(t)}
              className={`rounded-md border px-3 py-1.5 text-sm transition-colors ${
                theme === t
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border bg-card text-foreground hover:bg-accent/50'
              }`}
            >
              {t === 'dark' ? '🌙 Dark' : t === 'light' ? '☀️ Light' : '💻 System'}
            </button>
          ))}
        </div>
      </div>

      {/* IDE Configuration */}
      <div className="space-y-2">
        <Label>IDE</Label>
        <IDEConfig />
      </div>

      {/* Divider */}
      <div className="border-t border-border" />

      {/* Sync Interval */}
      <div className="space-y-2">
        <Label>Default Sync Interval</Label>
        <div className="flex items-center gap-2">
          <Input
            value={syncInterval}
            onChange={(e) => setSyncInterval(e.target.value)}
            placeholder="e.g. 30m, 1h, 2h"
            className="max-w-[200px]"
            disabled={isLoading}
          />
          {saving && <span className="text-xs text-muted-foreground">Saving...</span>}
        </div>
        <p className="text-xs text-muted-foreground">
          Go duration format: 30m, 1h, 2h30m
        </p>
      </div>

      {/* Sync on Startup */}
      <div className="space-y-2">
        <Toggle
          label="Sync on Startup"
          checked={engineConfig?.Sync?.SyncOnStartup ?? false}
          onChange={handleSyncOnStartup}
          disabled={isLoading}
        />
        <p className="text-xs text-muted-foreground">
          Automatically sync all repos when the app starts
        </p>
      </div>

      {/* Auto Launch */}
      <div className="space-y-2">
        <Toggle
          label="Open at Login"
          checked={engineConfig?.Sync?.AutoLaunch ?? false}
          onChange={handleAutoLaunch}
          disabled={isLoading}
        />
        <p className="text-xs text-muted-foreground">
          Launch ForkSync automatically when you log in
        </p>
      </div>

      {/* Divider */}
      <div className="border-t border-border" />

      {/* About */}
      <div className="space-y-1">
        <Label className="text-muted-foreground">About</Label>
        <p className="text-xs text-muted-foreground">
          ForkSync — Keep your fork repositories up to date
        </p>
        <p className="text-xs text-muted-foreground">Config: ~/.forksync/config.yaml</p>
        <p className="text-xs text-muted-foreground">Data: ~/.forksync/repos.json</p>
      </div>
    </div>
  )
}
