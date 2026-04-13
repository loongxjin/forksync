import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { useTranslation } from 'react-i18next'

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
