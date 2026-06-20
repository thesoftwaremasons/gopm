import { Link, useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listJoins, deleteJoin } from '../api/client'

export function JoinList() {
  const { joinId } = useParams<{ joinId: string }>()
  const qc = useQueryClient()

  const { data: joins = [], isLoading } = useQuery({
    queryKey: ['joins'],
    queryFn: listJoins,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteJoin,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['joins'] }),
  })

  if (isLoading) {
    return <div className="px-4 py-2 text-xs text-slate-400">Loading…</div>
  }

  return (
    <div className="py-1">
      {joins.length === 0 && (
        <div className="px-4 py-2 text-xs text-slate-500 italic">No joins yet</div>
      )}
      {joins.map((j) => (
        <div
          key={j.id}
          className={`group flex items-center gap-1 px-3 py-1 rounded mx-1 transition-colors ${
            joinId === j.id ? 'bg-slate-600 text-white' : 'text-slate-300 hover:bg-slate-700'
          }`}
        >
          <Link
            to={`/joins/${j.id}`}
            className="flex-1 text-xs truncate"
            title={j.name}
          >
            🔗 {j.name}
          </Link>
          <button
            onClick={(e) => {
              e.preventDefault()
              if (confirm(`Delete join "${j.name}"?`)) {
                deleteMutation.mutate(j.id)
              }
            }}
            className="opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-300 text-xs transition-opacity"
            title="Delete join"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
