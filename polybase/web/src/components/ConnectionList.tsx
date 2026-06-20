import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { listConnections } from '../api/client'

export function ConnectionList() {
  const { id: activeId } = useParams()
  const { data: connections = [], isLoading, error } = useQuery({
    queryKey: ['connections'],
    queryFn: listConnections,
  })

  if (isLoading) return <div className="p-3 text-slate-400 text-sm">Loading...</div>
  if (error) return <div className="p-3 text-red-400 text-sm">Error loading connections</div>

  return (
    <div>
      <div className="px-3 py-2 text-xs font-semibold text-slate-400 uppercase tracking-wider">
        Connections
      </div>
      {connections.length === 0 && (
        <div className="px-3 py-2 text-slate-500 text-sm">No connections yet</div>
      )}
      {connections.map((conn) => (
        <Link
          key={conn.id}
          to={`/connections/${conn.id}/browse`}
          className={`flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-slate-700 transition-colors ${
            conn.id === activeId ? 'bg-slate-700 text-white' : 'text-slate-300'
          }`}
        >
          <span className="text-base">{conn.engine === 'postgres' ? '🐘' : '🍃'}</span>
          <span className="truncate">{conn.name}</span>
        </Link>
      ))}
    </div>
  )
}
