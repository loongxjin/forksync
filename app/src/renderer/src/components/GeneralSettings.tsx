import { useState, useEffect, useRef } from 'react'
import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { IDEConfig } from '@/components/IDEConfig'
import { useTranslation } from 'react-i18next'

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
  const { t } = useTranslation()

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
        <Label>{t('settings.general.theme')}</Label>
        <div className="flex gap-2">
          {(['dark', 'light', 'system'] as const).map((themeVal) => (
            <button
              key={themeVal}
              onClick={() => setTheme(themeVal)}
              className={`rounded-md border px-3 py-1.5 text-sm transition-colors ${
                theme === themeVal
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border bg-card text-foreground hover:bg-accent/50'
              }`}
            >
              {themeVal === 'dark' ? '🌙 ' + t('theme.dark') : themeVal === 'light' ? '☀️ ' + t('theme.light') : '💻 ' + t('theme.system')}
            </button>
          ))}
        </div>
      </div>

      {/* IDE Configuration */}
      <div className="space-y-2">
        <Label>{t('settings.general.ide')}</Label>
        <IDEConfig />
      </div>

      {/* Divider */}
      <div className="border-t border-border" />

      {/* Sync Interval */}
      <div className="space-y-2">
        <Label>{t('settings.general.defaultSyncInterval')}</Label>
        <div className="flex items-center gap-2">
          <Input
            value={syncInterval}
            onChange={(e) => setSyncInterval(e.target.value)}
            placeholder={t('settings.general.syncIntervalPlaceholder')}
            className="max-w-[200px]"
            disabled={isLoading}
          />
          {saving && <span className="text-xs text-muted-foreground">{t('common.saving')}</span>}
        </div>
        <p className="text-xs text-muted-foreground">
          {t('settings.general.syncIntervalHint')}
        </p>
      </div>

      {/* Sync on Startup */}
      <div className="space-y-2">
        <Toggle
          label={t('settings.general.syncOnStartup')}
          checked={engineConfig?.Sync?.SyncOnStartup ?? false}
          onChange={handleSyncOnStartup}
          disabled={isLoading}
        />
        <p className="text-xs text-muted-foreground">
          {t('settings.general.syncOnStartupDesc')}
        </p>
      </div>

      {/* Auto Launch */}
      <div className="space-y-2">
        <Toggle
          label={t('settings.general.openAtLogin')}
          checked={engineConfig?.Sync?.AutoLaunch ?? false}
          onChange={handleAutoLaunch}
          disabled={isLoading}
        />
        <p className="text-xs text-muted-foreground">
          {t('settings.general.openAtLoginDesc')}
        </p>
      </div>

      {/* Divider */}
      <div className="border-t border-border" />

      {/* About */}
      <div className="space-y-1">
        <Label className="text-muted-foreground">{t('settings.general.about')}</Label>
        <p className="text-xs text-muted-foreground">
          {t('settings.general.aboutText')}
        </p>
        <p className="text-xs text-muted-foreground">{t('settings.general.configPath')}</p>
        <p className="text-xs text-muted-foreground">{t('settings.general.dataPath')}</p>
      </div>
    </div>
  )
}
