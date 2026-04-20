import { useTranslation } from 'react-i18next'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { GeneralSettings } from '@/components/GeneralSettings'
import { AgentConfig } from '@/components/AgentConfig'
import { NotificationSettings } from '@/components/NotificationSettings'
import { ProxySettings } from '@/components/ProxySettings'
import { Separator } from '@/components/ui/separator'

interface SettingsDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SettingsDrawer({ open, onOpenChange }: SettingsDrawerProps): JSX.Element {
  const { t } = useTranslation()

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[340px] overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{t('settings.title')}</SheetTitle>
        </SheetHeader>

        <div className="px-6 py-4 space-y-6">
          {/* General */}
          <section>
            <h3 className="text-sm font-medium text-muted-foreground mb-3">
              {t('settings.tabs.general')}
            </h3>
            <GeneralSettings />
          </section>

          <Separator />

          {/* Agent */}
          <section>
            <h3 className="text-sm font-medium text-muted-foreground mb-3">
              {t('settings.tabs.agent')}
            </h3>
            <AgentConfig />
          </section>

          <Separator />

          {/* Advanced: Notification + Proxy */}
          <section>
            <h3 className="text-sm font-medium text-muted-foreground mb-3">
              {t('settings.tabs.notification')} / {t('settings.tabs.proxy')}
            </h3>
            <div className="space-y-6">
              <NotificationSettings />
              <ProxySettings />
            </div>
          </section>
        </div>
      </SheetContent>
    </Sheet>
  )
}
