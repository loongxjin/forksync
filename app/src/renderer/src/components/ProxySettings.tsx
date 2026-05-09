import { useState, useEffect, useRef } from 'react'
import { useSettings } from '@/contexts/SettingsContext'
import { Toggle } from '@/components/ui/toggle'
import { Input } from '@/components/ui/input'
import { useTranslation } from 'react-i18next'



export function ProxySettings(): JSX.Element {
  const { engineConfig, configLoading, updateConfig } = useSettings()
  const { t } = useTranslation()

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
        label={t('settings.proxy.enable')}
        checked={proxyEnabled}
        onChange={handleToggleProxy}
        disabled={isLoading}
      />
      <p className="text-xs text-muted-foreground -mt-4">
        {t('settings.proxy.description')}
      </p>

      <div className="border-t border-border" />

      <div className="space-y-2">
        <Label>{t('settings.proxy.proxyUrl')}</Label>
        <div className="flex items-center gap-2">
          <Input
            value={proxyUrl}
            onChange={(e) => setProxyUrl(e.target.value)}
            placeholder={t('settings.proxy.proxyUrlPlaceholder')}
            className="max-w-[320px]"
            disabled={isLoading || !proxyEnabled}
          />
          {saving && <span className="text-xs text-muted-foreground">{t('common.saving')}</span>}
        </div>
        <p className="text-xs text-muted-foreground">
          {t('settings.proxy.supports')}
        </p>
      </div>
    </div>
  )
}
