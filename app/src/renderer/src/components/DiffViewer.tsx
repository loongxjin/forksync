import { useMemo } from 'react'

interface DiffViewerProps {
  diff: string
  className?: string
}

export function DiffViewer({ diff, className }: DiffViewerProps): JSX.Element {
  const lines = useMemo(() => {
    if (!diff) return []
    return diff.split('\n').map((line, i) => ({
      num: i + 1,
      content: line,
      type: getDiffLineType(line)
    }))
  }, [diff])

  if (!diff) {
    return <p className="text-sm text-muted-foreground">No diff available.</p>
  }

  return (
    <div
      className={`overflow-x-auto rounded-md border border-border bg-background font-mono text-xs ${className ?? ''}`}
    >
      <table className="w-full border-collapse">
        <tbody>
          {lines.map((line) => (
            <tr key={line.num} className={getLineClasses(line.type)}>
              <td className="w-10 select-none px-2 py-0.5 text-right text-muted-foreground/50">
                {line.num}
              </td>
              <td className="whitespace-pre-wrap break-all px-2 py-0.5">
                {line.content || ' '}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

type DiffLineType = 'add' | 'remove' | 'header' | 'normal'

function getDiffLineType(line: string): DiffLineType {
  if (line.startsWith('+++') || line.startsWith('---')) return 'header'
  if (line.startsWith('@@')) return 'header'
  if (line.startsWith('+')) return 'add'
  if (line.startsWith('-')) return 'remove'
  return 'normal'
}

function getLineClasses(type: DiffLineType): string {
  switch (type) {
    case 'add':
      return 'bg-emerald-500/10 text-emerald-400'
    case 'remove':
      return 'bg-red-500/10 text-red-400'
    case 'header':
      return 'bg-blue-500/10 text-blue-400'
    default:
      return ''
  }
}
