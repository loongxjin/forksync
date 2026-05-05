import { useEffect, useState } from 'react'
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
import { FileDiff, X } from 'lucide-react'

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
  }, [open, repoName, t])

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
          {error && (
            <div className="text-sm text-error text-center py-8">{error}</div>
          )}
          {!loading && !error && (
            <DiffViewer diff={diff} />
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
