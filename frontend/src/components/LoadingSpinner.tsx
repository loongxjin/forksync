export function LoadingSpinner({ className }: { className?: string }): JSX.Element {
  return (
    <div className={`flex items-center justify-center py-8 ${className ?? ''}`}>
      <div className="h-5 w-5 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
    </div>
  )
}
