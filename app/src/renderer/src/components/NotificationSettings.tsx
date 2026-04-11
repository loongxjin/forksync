import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'

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

  const isLoading = configLoading || !engineConfig

  const handleToggle = async (key: string, val: boolean): Promise<void> => {
    await updateConfig(key, String(val))
  }

  return (
    <div className="space-y-6">
      <Toggle
        label="Enable Notifications"
        checked={engineConfig?.Notification?.Enabled ?? true}
        onChange={(val) => handleToggle('notification.enabled', val)}
        disabled={isLoading}
      />
      <p className="text-xs text-muted-foreground -mt-4">
        Show system notifications for sync events
      </p>

      <div className="border-t border-border" />

      <Toggle
        label="Notify on Conflict"
        checked={engineConfig?.Notification?.OnConflict ?? true}
        onChange={(val) => handleToggle('notification.on_conflict', val)}
        disabled={isLoading}
      />
      <p className="text-xs text-muted-foreground -mt-4">
        Show notification when a conflict requires manual resolution
      </p>

      <Toggle
        label="Notify on Sync Success"
        checked={engineConfig?.Notification?.OnSyncSuccess ?? false}
        onChange={(val) => handleToggle('notification.on_sync_success', val)}
        disabled={isLoading}
      />
      <p className="text-xs text-muted-foreground -mt-4">
        Show notification when sync completes successfully with new commits
      </p>
    </div>
  )
}
