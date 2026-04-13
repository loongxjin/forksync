/**
 * IDEOpenButton — opens a repo in the default IDE, with dropdown for alternatives
 */

import { useState, useRef, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useSettings } from '@/contexts/SettingsContext'
import type { IDEInfo } from '@/types/ide'

interface IDEOpenButtonProps {
  repoPath: string
}

export function IDEOpenButton({ repoPath }: IDEOpenButtonProps): JSX.Element {
  const { t } = useTranslation()
  const { getInstalledIDEs, getDefaultIDE, setDefaultIDE, openInIDE, ideLoading } = useSettings()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [opening, setOpening] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  const defaultIDE = getDefaultIDE()
  const installedIDEs = getInstalledIDEs()

  // Close dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  const handleOpen = useCallback(
    async (ide: IDEInfo) => {
      setOpening(true)
      const result = await openInIDE(repoPath, ide.id)
      setOpening(false)
      setOpen(false)
      if (!result.success) {
        alert(result.error ?? t('ide.openFailed'))
      }
    },
    [repoPath, openInIDE, t]
  )

  const handleMainClick = useCallback(() => {
    if (defaultIDE) {
      handleOpen(defaultIDE)
    } else {
      setOpen(!open)
    }
  }, [defaultIDE, open, handleOpen])

  const handleSettingsClick = useCallback(() => {
    setOpen(false)
    navigate('/settings')
  }, [navigate])

  if (ideLoading) return <></>

  if (installedIDEs.length === 0) return <></>

  return (
    <div className="relative" ref={dropdownRef}>
      <div className="flex">
        <button
          onClick={handleMainClick}
          disabled={opening}
          className="rounded-l px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
          title={defaultIDE ? t('ide.openWith', { name: defaultIDE.name }) : t('ide.selectIdeToOpen')}
        >
          {opening ? '⏳' : '<>'} {defaultIDE ? defaultIDE.name : t('ide.open')}
        </button>
        {installedIDEs.length > 1 && (
          <button
            onClick={() => setOpen(!open)}
            className="rounded-r border-l border-border px-1 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
            title={t('ide.selectOtherIde')}
          >
            ▾
          </button>
        )}
      </div>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 min-w-[180px] rounded-md border border-border bg-card py-1 shadow-lg">
          {installedIDEs.map((ide) => (
            <button
              key={ide.id}
              onClick={() => handleOpen(ide)}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-foreground hover:bg-accent"
            >
              {ide.id === defaultIDE?.id && <span className="text-primary">✓</span>}
              {ide.id !== defaultIDE?.id && <span className="w-3" />}
              {ide.name}
              {ide.id === defaultIDE?.id && (
                <span className="text-muted-foreground">{t('ide.default')}</span>
              )}
            </button>
          ))}
          <div className="my-1 border-t border-border" />
          <button
            onClick={handleSettingsClick}
            className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            {t('ide.setDefault')}
          </button>
        </div>
      )}
    </div>
  )
}
