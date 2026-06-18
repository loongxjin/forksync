import { useCallback, useRef, useState, type DragEvent } from 'react'
import { engineApi } from '@/lib/api'
import { useRepos } from '@/contexts/RepoContext'
import { useLogger } from '@/hooks/useLogger'

/**
 * useDragDropAdd — folder drag-and-drop → add repo / open scan dialog.
 *
 * Extracted from HomePage so the drag-counter ref + four event handlers + the
 * isGitRepo probe live in one testable unit. When a dropped folder is not a git
 * repo, `onScanNeeded(dir)` is called so the host can open the scan dialog
 * seeded with that directory.
 *
 * Returns the current `dragOver` state (for the overlay highlight) and a
 * `handlers` object spread onto the drop target element.
 */
export interface UseDragDropAddHandlers {
  onDragEnter: (e: DragEvent<HTMLDivElement>) => void
  onDragOver: (e: DragEvent<HTMLDivElement>) => void
  onDragLeave: (e: DragEvent<HTMLDivElement>) => void
  onDrop: (e: DragEvent<HTMLDivElement>) => void
}

export function useDragDropAdd(onScanNeeded: (dir: string) => void): {
  dragOver: boolean
  handlers: UseDragDropAddHandlers
} {
  const logger = useLogger('useDragDropAdd')
  const { addRepo } = useRepos()
  const [dragOver, setDragOver] = useState(false)
  // Counter prevents flicker when the pointer crosses into child elements.
  const dragCounterRef = useRef(0)

  const handleDragEnter = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer.types.includes('Files')) {
      dragCounterRef.current++
      setDragOver(true)
    }
  }, [])

  const handleDragOver = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const handleDragLeave = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    dragCounterRef.current--
    if (dragCounterRef.current === 0) setDragOver(false)
  }, [])

  const handleDrop = useCallback(
    async (e: DragEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.stopPropagation()
      setDragOver(false)

      const files = e.dataTransfer.files
      for (let i = 0; i < files.length; i++) {
        const file = files[i]
        const path = (file as File & { path?: string }).path
        if (!path) continue
        try {
          const isGit = await engineApi.isGitRepo(path)
          if (isGit) {
            await addRepo(path)
          } else {
            onScanNeeded(path)
          }
        } catch (err) {
          logger.error('drop add failed', err)
        }
      }
    },
    [addRepo, onScanNeeded, logger]
  )

  return {
    dragOver,
    handlers: { onDragEnter: handleDragEnter, onDragOver: handleDragOver, onDragLeave: handleDragLeave, onDrop: handleDrop }
  }
}
