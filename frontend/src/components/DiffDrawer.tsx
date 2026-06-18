import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetClose
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { DiffViewer } from '@/components/DiffViewer'
import { engineApi } from '@/lib/api'
import { FileDiff, X, AlertCircle, RotateCw } from 'lucide-react'

interface DiffDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  repoName: string
}

export function DiffDrawer({ open, onOpenChange, repoName }: DiffDrawerProps): JSX.Element {
  const { t } = useTranslation()
  const [diff, setDiff] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  // Bumped by the Retry button to re-trigger the fetch effect.
  const [reloadKey, setReloadKey] = useState(0)

  const reload = useCallback(() => setReloadKey((k) => k + 1), [])

  useEffect(() => {
    if (!open || !repoName) {
      setDiff('')
      setError('')
      return
    }
    setLoading(true)
    setError('')
    engineApi.repoDiff(repoName)
      .then((res) => {
        if (res.success && res.diff != null) {
          setDiff(res.diff)
        } else {
          setError(res.error ?? t('diffViewer.failed'))
        }
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- t is stable from react-i18next; including it causes re-fetches
  }, [open, repoName, reloadKey])

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[700px] max-w-full flex flex-col">
        <SheetHeader className="shrink-0 border-b px-4 py-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <SheetTitle className="text-sm font-medium flex items-center gap-2">
                <FileDiff size={14} />
                {t('diffViewer.title')}
              </SheetTitle>
              <span className="text-xs text-muted-foreground">— {repoName}</span>
            </div>
            <SheetClose asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7">
                <X size={14} />
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto p-4">
          {loading && (
            <div className="text-sm text-muted-foreground text-center py-8">
              {t('common.loading')}
            </div>
          )}
          {!loading && error && (
            <div className="flex flex-col items-center gap-3 py-10 text-center">
              <AlertCircle size={28} className="text-error" />
              <div className="text-sm text-error max-w-md break-all">{error}</div>
              <Button onClick={reload} disabled={loading} size="sm" variant="outline" className="text-xs">
                <RotateCw size={14} className="mr-1.5" />
                {t('common.retry')}
              </Button>
            </div>
          )}
          {!loading && !error && !diff && (
            <div className="text-sm text-muted-foreground text-center py-10">
              {t('diffViewer.emptyHint')}
            </div>
          )}
          {!loading && !error && diff && (
            <DiffViewer diff={diff} />
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
