import { useParams } from 'react-router-dom'

export function ConflictDetail(): JSX.Element {
  const { repoId } = useParams<{ repoId: string }>()

  return (
    <div>
      <h2 className="text-xl font-semibold">Conflict: {repoId}</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        Resolve merge conflicts for this repository.
      </p>
    </div>
  )
}
