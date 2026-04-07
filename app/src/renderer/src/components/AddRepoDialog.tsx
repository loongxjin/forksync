import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'

interface AddRepoDialogProps {
  open: boolean
  onClose: () => void
  onAdd: (path: string, upstream?: string) => Promise<void>
}

export function AddRepoDialog({ open, onClose, onAdd }: AddRepoDialogProps): JSX.Element | null {
  const [path, setPath] = useState('')
  const [upstream, setUpstream] = useState('')
  const [adding, setAdding] = useState(false)

  if (!open) return null

  const handleSubmit = async (e: React.FormEvent): Promise<void> => {
    e.preventDefault()
    if (!path.trim()) return
    setAdding(true)
    try {
      await onAdd(path.trim(), upstream.trim() || undefined)
      setPath('')
      setUpstream('')
      onClose()
    } finally {
      setAdding(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-6 shadow-lg">
        <h3 className="text-lg font-semibold">Add Repository</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Enter the local path to a git repository.
        </p>

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <div className="space-y-2">
            <Label htmlFor="repo-path">Repository Path</Label>
            <Input
              id="repo-path"
              placeholder="/path/to/your/repo"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="upstream">
              Upstream URL <span className="text-muted-foreground">(optional, auto-detected for forks)</span>
            </Label>
            <Input
              id="upstream"
              placeholder="https://github.com/original/repo.git"
              value={upstream}
              onChange={(e) => setUpstream(e.target.value)}
            />
          </div>

          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!path.trim() || adding}>
              {adding ? 'Adding...' : 'Add'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
