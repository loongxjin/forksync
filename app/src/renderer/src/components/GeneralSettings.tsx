import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'

export function GeneralSettings(): JSX.Element {
  const { theme, setTheme } = useSettings()

  return (
    <div className="space-y-4">
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

      <div className="space-y-2">
        <Label>Sync Interval</Label>
        <p className="text-xs text-muted-foreground">
          Default interval is configured in ~/.forksync/config.yaml
        </p>
      </div>

      <div className="space-y-2">
        <Label>About</Label>
        <p className="text-xs text-muted-foreground">
          ForkSync — Keep your fork repositories up to date
        </p>
        <p className="text-xs text-muted-foreground">
          Config: ~/.forksync/config.yaml
        </p>
        <p className="text-xs text-muted-foreground">
          Data: ~/.forksync/repos.json
        </p>
      </div>
    </div>
  )
}
