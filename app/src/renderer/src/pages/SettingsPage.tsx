/**
 * SettingsPage — standalone tabbed settings (cc-switch style).
 *
 * Replaces the old right-slide SettingsDrawer. The four sub-components
 * (GeneralSettings, AgentConfig, NotificationSettings, ProxySettings) are
 * self-contained and reused unchanged.
 */

import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, Settings, Bot, Bell, Globe } from 'lucide-react'
import { GeneralSettings } from '@/components/GeneralSettings'
import { AgentConfig } from '@/components/AgentConfig'
import { NotificationSettings } from '@/components/NotificationSettings'
import { ProxySettings } from '@/components/ProxySettings'

type TabKey = 'general' | 'agent' | 'notification' | 'proxy'

interface Tab {
  key: TabKey
  icon: typeof Settings
  label: string
}

const TABS: Tab[] = [
  { key: 'general', icon: Settings, label: 'settings.tabs.general' },
  { key: 'agent', icon: Bot, label: 'settings.tabs.agent' },
  { key: 'notification', icon: Bell, label: 'settings.tabs.notification' },
  { key: 'proxy', icon: Globe, label: 'settings.tabs.proxy' }
]

export function SettingsPage(): JSX.Element {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<TabKey>('general')

  return (
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex items-center gap-3 mb-5">
        <button
          onClick={() => navigate('/')}
          className="flex items-center gap-0.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ChevronLeft size={16} />
          {t('common.back')}
        </button>
        <h1 className="text-lg font-semibold">{t('settings.title')}</h1>
      </div>

      {/* Tab rail + content */}
      <div className="flex gap-6">
        {/* Left tab rail */}
        <nav className="w-40 shrink-0 space-y-0.5">
          {TABS.map((tab) => {
            const Icon = tab.icon
            const isActive = activeTab === tab.key
            return (
              <button
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                className={`flex items-center gap-2 w-full text-left px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive
                    ? 'bg-accent text-foreground font-medium'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent/50'
                }`}
              >
                <Icon size={16} />
                {t(tab.label)}
              </button>
            )
          })}
        </nav>

        {/* Right content */}
        <div className="flex-1 min-w-0">
          {activeTab === 'general' && <GeneralSettings />}
          {activeTab === 'agent' && <AgentConfig />}
          {activeTab === 'notification' && <NotificationSettings />}
          {activeTab === 'proxy' && <ProxySettings />}
        </div>
      </div>
    </div>
  )
}
