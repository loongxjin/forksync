import { Component, type ErrorInfo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

interface ErrorBoundaryProps {
  children: ReactNode
  fallback?: ReactNode
}

interface ErrorBoundaryState {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    console.error('ErrorBoundary caught:', error, errorInfo)
  }

  render(): ReactNode {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback
      }
      return <ErrorBoundaryFallback error={this.state.error} onRetry={() => this.setState({ hasError: false, error: null })} />
    }

    return this.props.children
  }
}

function ErrorBoundaryFallback({
  error,
  onRetry
}: {
  error: Error | null
  onRetry: () => void
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-destructive/50 bg-destructive/5 p-6">
      <p className="text-sm font-medium text-destructive">{t('errorBoundary.title')}</p>
      <p className="max-w-md text-center text-xs text-muted-foreground">
        {error?.message ?? t('errorBoundary.message')}
      </p>
      <button
        onClick={onRetry}
        className="rounded-md border border-border px-3 py-1 text-xs hover:bg-accent"
      >
        {t('errorBoundary.retry')}
      </button>
    </div>
  )
}
