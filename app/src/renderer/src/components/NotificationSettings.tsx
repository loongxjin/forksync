import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { Toggle } from '@/components/ui/toggle'
import { useTranslation } from 'react-i18next'

export function NotificationSettings(): JSX.Element {
  const { engineConfig, configLoading, updateConfig } = useSettings()
  const { t } = useTranslation()

  const isLoading = configLoading || !engineConfig

  return (
    <div className="space-y-6">
      <Toggle
        label={t('settings.notification.enable')}
        checked={engineConfig?.Notification?.Enabled ?? true}
        onChange={(val) => updateConfig('notification.enabled', String(val))}
        disabled={isLoading}
      />
      <p className="text-xs text-muted-foreground -mt-4">
        {t('settings.notification.description')}
      </p>
    </div>
  )
}
