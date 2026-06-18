import { useState, useEffect } from 'react'
import { useSettings } from '@/contexts/SettingsContext'
import { Label } from '@/components/ui/label'
import { Toggle } from '@/components/ui/toggle'
import { Input } from '@/components/ui/input'
import { useTranslation } from 'react-i18next'
import { useDebouncedConfig } from '@/hooks/useDebouncedConfig'



export function ProxySettings(): JSX.Element {
  const { engineConfig, configLoading, updateConfig } = useSettings()
  const { t } = useTranslation()

  // Debounced config input for proxy URL
  const [proxyUrl, setProxyUrl, saving] = useDebouncedConfig(
    'proxy.url', engineConfig?.Proxy?.URL, updateConfig
  )

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
