/**
 * HistoryRow — single sync history record display
 *
 * Extracted from HomePage to reduce its size and improve testability.
 */

import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { CheckCircle2, Zap, XCircle } from 'lucide-react'
import type { SyncHistoryRecord } from '@/types/engine'

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

export function getHistoryConfig(status: string, t: TFunction): { icon: React.ReactNode; label: string } {
  switch (status) {
    case 'synced': return { icon: <CheckCircle2 size={14} className="text-success" />, label: t('status.upToDate') }
    case 'up_to_date': return { icon: <CheckCircle2 size={14} className="text-success" />, label: t('status.upToDate') }
    case 'conflict': return { icon: <Zap size={14} className="text-error" />, label: t('status.conflict') }
    case 'error': return { icon: <XCircle size={14} className="text-error" />, label: t('status.error') }
    default: return { icon: <span className="text-muted-foreground text-xs">•</span>, label: status }
  }
}

export function formatTimeAgo(dateStr: string | null, t: TFunction): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const now = new Date()
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (seconds < 60) return t('dashboard.justNow')
  if (seconds < 3600) return t('dashboard.minutesAgo', { count: Math.floor(seconds / 60) })
  if (seconds < 86400) return t('dashboard.hoursAgo', { count: Math.floor(seconds / 3600) })
  return t('dashboard.daysAgo', { count: Math.floor(seconds / 86400) })
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface HistoryRowProps {
  record: SyncHistoryRecord
  onRetry: (record: SyncHistoryRecord) => void
}

export function HistoryRow({ record, onRetry }: HistoryRowProps): JSX.Element {
  const { t } = useTranslation()
  const config = getHistoryConfig(record.status, t)
  const timeAgo = formatTimeAgo(record.createdAt, t)
  const [expanded, setExpanded] = useState(false)
  const [retrying, setRetrying] = useState(false)

  useEffect(() => {
    if (record.summaryStatus !== 'failed') {
      setRetrying(false)
    }
  }, [record.summaryStatus])

  const handleRetry = (): void => {
    if (retrying) return
    setRetrying(true)
    onRetry(record)
  }

  const shouldShowFull = (text: string): boolean => {
    return text.split('\n').length <= 3
  }

  const showSummary = record.summaryStatus === 'generating' || record.summaryStatus === 'pending' ||
    (record.summaryStatus === 'done' && record.summary) ||
    record.summaryStatus === 'failed'

  return (
    <div className="rounded-md px-2 py-1.5 text-sm hover:bg-accent/30 transition-colors duration-150">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          <span className="shrink-0">{config.icon}</span>
          <span className="font-medium truncate">{record.repoName}</span>
          <span className="text-muted-foreground">{config.label}</span>
          {record.commitsPulled > 0 && (
            <span className="text-xs text-muted-foreground tabular-nums">+{record.commitsPulled} commits</span>
          )}
          {record.agentUsed && (
            <span className="text-[10px] px-1.5 py-0.5 rounded-md bg-secondary text-secondary-foreground font-mono">
              {record.agentUsed}
            </span>
          )}
          {record.errorMessage && (
            <span className="truncate text-xs text-error min-w-0 max-w-[200px]" title={record.errorMessage}>
              {record.errorMessage}
            </span>
          )}
        </div>
        <span className="text-xs text-muted-foreground whitespace-nowrap ml-2">{timeAgo}</span>
      </div>

      {showSummary && (
        <div className="mt-1 ml-6">
          {record.summaryStatus === 'generating' || record.summaryStatus === 'pending' ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className="inline-block h-2.5 w-2.5 rounded-full bg-primary animate-pulse" />
              {t('summary.generating')}
            </div>
          ) : record.summaryStatus === 'done' && record.summary ? (
            <div className="text-xs text-muted-foreground leading-relaxed">
              {expanded || shouldShowFull(record.summary) ? (
                <>
                  {record.summary}
                  {!shouldShowFull(record.summary) && (
                    <button
                      onClick={() => setExpanded(false)}
                      className="ml-1 text-primary hover:underline"
                    >
                      {t('summary.collapse')}
                    </button>
                  )}
                </>
              ) : (
                <>
                  {record.summary.split('\n').slice(0, 3).join('\n')}...
                  <button
                    onClick={() => setExpanded(true)}
                    className="ml-1 text-primary hover:underline"
                  >
                    {t('summary.expand')}
                  </button>
                </>
              )}
            </div>
          ) : record.summaryStatus === 'failed' ? (
            <div className="flex items-center gap-2 text-xs">
              <span className="text-error">{t('summary.failed')}</span>
              <button
                onClick={handleRetry}
                disabled={retrying}
                className="text-primary hover:underline disabled:opacity-50"
              >
                {retrying ? t('common.processing') : t('summary.retry')}
              </button>
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}
