import { useState } from 'react'
import { GeneralSettings } from '@/components/GeneralSettings'
import { AgentConfig } from '@/components/AgentConfig'
import { NotificationSettings } from '@/components/NotificationSettings'
import { ProxySettings } from '@/components/ProxySettings'

const tabs = ['General', 'Agent', 'Notification', 'Proxy'] as const
type TabName = (typeof tabs)[number]

export function Settings(): JSX.Element {
  const [activeTab, setActiveTab] = useState<TabName>('General')

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Settings</h2>

      {/* Tab bar */}
      <div className="flex gap-1 border-b border-border">
        {tabs.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab
                ? 'border-b-2 border-primary text-foreground'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="mt-4">
        {activeTab === 'General' && <GeneralSettings />}
        {activeTab === 'Agent' && <AgentConfig />}
        {activeTab === 'Notification' && <NotificationSettings />}
        {activeTab === 'Proxy' && <ProxySettings />}
      </div>
    </div>
  )
}
