/**
 * IDEOpenButton — opens a repo in the default IDE
 */

import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings } from '@/contexts/SettingsContext'
import { useSettingsDrawer } from '@/contexts/SettingsDrawerContext'
import { Code, Loader2 } from 'lucide-react'

interface IDEOpenButtonProps {
  repoPath: string
}

export function IDEOpenButton({ repoPath }: IDEOpenButtonProps): JSX.Element {
  const { t } = useTranslation()
  const { getDefaultIDE, openInIDE, ideLoading } = useSettings()
  const { openDrawer } = useSettingsDrawer()
  const [opening, setOpening] = useState(false)

  const defaultIDE = getDefaultIDE()

  const handleOpen = useCallback(async () => {
    if (!defaultIDE) {
      openDrawer()
      return
    }
    setOpening(true)
    const result = await openInIDE(repoPath, defaultIDE.id)
    setOpening(false)
    if (!result.success) {
      alert(result.error ?? t('ide.openFailed'))
    }
  }, [repoPath, defaultIDE, openInIDE, openDrawer, t])

  if (ideLoading) return <></>

  // data-action: marker to prevent RepoRow expand toggle when clicking
  return (
    <button
      data-action
      onClick={handleOpen}
      disabled={opening}
      className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
      title={defaultIDE ? t('ide.openWith', { name: defaultIDE.name }) : t('ide.selectIdeToOpen')}
    >
      {opening ? <Loader2 size={14} className="animate-spin" /> : <Code size={14} />}
    </button>
  )
}
