import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { Toggle } from '@/components/ui/toggle'
import { Input } from '@/components/ui/input'
import { IDEConfig } from '@/components/IDEConfig'
import { useTranslation } from 'react-i18next'
import { Moon, Sun, Monitor } from 'lucide-react'
import { useDebouncedConfig } from '@/hooks/useDebouncedConfig'
import { engineApi } from '@/lib/api'

/** A simple toggle switch component */


export function GeneralSettings(): JSX.Element {
  const { theme, setTheme, engineConfig, configLoading, updateConfig } = useSettings()
  const { t } = useTranslation()

  // Debounced config input for sync interval
  const [syncInterval, setSyncInterval, saving] = useDebouncedConfig(
    'sync.default_interval', engineConfig?.Sync?.DefaultInterval ?? '', updateConfig
  )

  const handleSyncOnStartup = async (val: boolean): Promise<void> => {
    await updateConfig('sync.sync_on_startup', String(val))
  }

  const handleAutoLaunch = async (val: boolean): Promise<void> => {
    await updateConfig('sync.auto_launch', String(val))
    // Also update OS login item
    await engineApi.setAutoLaunch(val)
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
              <span className="flex items-center gap-1.5">
                {themeVal === 'dark' && <Moon size={14} />}
                {themeVal === 'light' && <Sun size={14} />}
                {themeVal === 'system' && <Monitor size={14} />}
                {themeVal === 'dark' ? t('theme.dark') : themeVal === 'light' ? t('theme.light') : t('theme.system')}
              </span>
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
