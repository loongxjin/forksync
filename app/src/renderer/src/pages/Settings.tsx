import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GeneralSettings } from '@/components/GeneralSettings'
import { AgentConfig } from '@/components/AgentConfig'
import { NotificationSettings } from '@/components/NotificationSettings'
import { ProxySettings } from '@/components/ProxySettings'

const tabKeys = ['general', 'agent', 'notification', 'proxy'] as const
type TabKey = (typeof tabKeys)[number]

export function Settings(): JSX.Element {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<TabKey>('general')

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">{t('settings.title')}</h2>

      {/* Tab bar */}
      <div className="flex gap-1 border-b border-border">
        {tabKeys.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab
                ? 'border-b-2 border-primary text-foreground'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            {t(`settings.tabs.${tab}`)}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="mt-4">
        {activeTab === 'general' && <GeneralSettings />}
        {activeTab === 'agent' && <AgentConfig />}
        {activeTab === 'notification' && <NotificationSettings />}
        {activeTab === 'proxy' && <ProxySettings />}
      </div>
    </div>
  )
}
