import { Modal } from '@/components/ui/modal'

interface ConfirmDialogProps {
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  confirmVariant?: 'default' | 'destructive'
  onConfirm: () => void
  onCancel: () => void
}

/**
 * Styled confirmation dialog replacing native confirm(). Uses the shared Modal
 * wrapper (with focus trap, Escape, scroll lock) for consistent look and
 * keyboard accessibility.
 */
export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  confirmVariant = 'default',
  onConfirm,
  onCancel
}: ConfirmDialogProps): JSX.Element {
  const isDestructive = confirmVariant === 'destructive'

  return (
    <Modal open={open} onClose={onCancel} maxWidth="max-w-sm">
      <div className="space-y-4">
        <h2 className="text-lg font-semibold text-foreground">{title}</h2>
        <p className="text-sm text-muted-foreground">{message}</p>
        <div className="flex justify-end gap-3 pt-2">
          <button
            onClick={onCancel}
            className="rounded-md border border-border px-4 py-2 text-sm text-foreground hover:bg-muted transition-colors"
          >
            {cancelLabel}
          </button>
          <button
            onClick={onConfirm}
            className={`rounded-md px-4 py-2 text-sm font-medium text-white transition-colors ${
              isDestructive
                ? 'bg-error hover:bg-error/90'
                : 'bg-primary hover:bg-primary/90'
            }`}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </Modal>
  )
}
