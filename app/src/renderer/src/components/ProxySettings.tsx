import { useState, useEffect, useRef } from 'react'
import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'

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

export function ProxySettings(): JSX.Element {
  const { engineConfig, configLoading, updateConfig } = useSettings()

  const [proxyUrl, setProxyUrl] = useState('socks5://127.0.0.1:7890')
  const [saving, setSaving] = useState(false)
  const isEditingRef = useRef(false)
  const prevConfigUrlRef = useRef('')

  useEffect(() => {
    if (engineConfig?.Proxy?.URL !== undefined) {
      // Only sync from config if user is NOT actively editing
      // and the config value actually changed from what we last synced
      if (!isEditingRef.current && engineConfig.Proxy.URL !== prevConfigUrlRef.current) {
        setProxyUrl(engineConfig.Proxy.URL)
        prevConfigUrlRef.current = engineConfig.Proxy.URL
      }
    }
  }, [engineConfig])

  // Debounced save for proxy URL
  useEffect(() => {
    if (!engineConfig) return
    if (proxyUrl === engineConfig.Proxy?.URL) return
    if (!engineConfig.Proxy?.Enabled) return // only save URL if proxy is enabled

    isEditingRef.current = true
    const timer = setTimeout(async () => {
      setSaving(true)
      await updateConfig('proxy.url', proxyUrl)
      setSaving(false)
      isEditingRef.current = false
      prevConfigUrlRef.current = proxyUrl
    }, 1500)

    return () => clearTimeout(timer)
  }, [proxyUrl, engineConfig, updateConfig])

  const handleToggleProxy = async (val: boolean): Promise<void> => {
    await updateConfig('proxy.enabled', String(val))
  }

  const isLoading = configLoading || !engineConfig
  const proxyEnabled = engineConfig?.Proxy?.Enabled ?? false

  return (
    <div className="space-y-6">
      <Toggle
        label="Enable Proxy"
        checked={proxyEnabled}
        onChange={handleToggleProxy}
        disabled={isLoading}
      />
      <p className="text-xs text-muted-foreground -mt-4">
        Route GitHub API requests through a proxy (HTTP/SOCKS5)
      </p>

      <div className="border-t border-border" />

      <div className="space-y-2">
        <Label>Proxy URL</Label>
        <div className="flex items-center gap-2">
          <Input
            value={proxyUrl}
            onChange={(e) => setProxyUrl(e.target.value)}
            placeholder="e.g. socks5://127.0.0.1:7890"
            className="max-w-[320px]"
            disabled={isLoading || !proxyEnabled}
          />
          {saving && <span className="text-xs text-muted-foreground">Saving...</span>}
        </div>
        <p className="text-xs text-muted-foreground">
          Supports HTTP and SOCKS5 proxies
        </p>
      </div>
    </div>
  )
}
