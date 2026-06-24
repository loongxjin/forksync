import { useTranslation } from 'react-i18next'
import { X, RefreshCw } from 'lucide-react'

interface ErrorBannerProps {
  message: string
  onRetry?: () => void
  onDismiss?: () => void
}

/**
 * Persistent error banner with optional retry and dismiss controls.
 * Used for context-level errors (repos.error, agents.error) that need user
 * action, as opposed to ephemeral operation errors shown via toast.
 */
export function ErrorBanner({ message, onRetry, onDismiss }: ErrorBannerProps): JSX.Element {
  const { t } = useTranslation()
  return (
    <div
      role="alert"
      className="flex items-center gap-3 rounded-lg border border-error/30 bg-error-muted px-4 py-3 text-sm text-error animate-fade-in"
    >
      <span className="flex-1">{message}</span>
      <div className="flex gap-1">
        {onRetry && (
          <button
            onClick={onRetry}
            className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-medium text-error hover:bg-error/10 transition-colors"
            aria-label={t('common.retry')}
          >
            <RefreshCw size={14} />
            {t('common.retry')}
          </button>
        )}
        {onDismiss && (
          <button
            onClick={onDismiss}
            className="inline-flex items-center rounded p-1 text-error/70 hover:text-error hover:bg-error/10 transition-colors"
            aria-label={t('common.dismiss')}
          >
            <X size={14} />
          </button>
        )}
      </div>
    </div>
  )
}
