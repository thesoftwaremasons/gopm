import { Link, useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listCharts, deleteChart } from '../api/client'

export function ChartList() {
  const { chartId } = useParams<{ chartId: string }>()
  const qc = useQueryClient()

  const { data: charts = [], isLoading } = useQuery({
    queryKey: ['charts'],
    queryFn: listCharts,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteChart,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['charts'] }),
  })

  if (isLoading) {
    return <div className="px-4 py-2 text-xs text-slate-400">Loading…</div>
  }

  return (
    <div className="py-1">
      {charts.length === 0 && (
        <div className="px-4 py-2 text-xs text-slate-500 italic">No charts yet</div>
      )}
      {charts.map((c) => (
        <div
          key={c.id}
          className={`group flex items-center gap-1 px-3 py-1 rounded mx-1 transition-colors ${
            chartId === c.id ? 'bg-slate-600 text-white' : 'text-slate-300 hover:bg-slate-700'
          }`}
        >
          <Link to={`/charts/${c.id}`} className="flex-1 text-xs truncate" title={c.name}>
            📊 {c.name}
          </Link>
          <button
            onClick={(e) => {
              e.preventDefault()
              if (confirm(`Delete chart "${c.name}"?`)) deleteMutation.mutate(c.id)
            }}
            className="opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-300 text-xs transition-opacity"
            title="Delete chart"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
