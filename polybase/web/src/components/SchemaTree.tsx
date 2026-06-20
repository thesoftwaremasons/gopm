import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { listSchemas } from '../api/client'

export function SchemaTree() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const { data: schemas = [], isLoading, error } = useQuery({
    queryKey: ['schemas', id],
    queryFn: () => listSchemas(id!),
    enabled: !!id,
  })

  if (!id) return null
  if (isLoading) return <div className="p-3 text-slate-400 text-sm">Loading schema...</div>
  if (error) return <div className="p-3 text-red-400 text-sm">Failed to load schema</div>

  const toggle = (name: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <div className="mt-2">
      <div className="px-3 py-1 text-xs font-semibold text-slate-400 uppercase tracking-wider">
        Schema
      </div>
      {schemas.map((schema) => (
        <div key={schema.name}>
          <button
            onClick={() => toggle(schema.name)}
            className="w-full flex items-center gap-1 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-700 transition-colors"
          >
            <span className="text-xs">{expanded.has(schema.name) ? '▼' : '▶'}</span>
            <span className="font-medium">{schema.name}</span>
            <span className="ml-auto text-xs text-slate-500">{schema.tables?.length ?? 0}</span>
          </button>
          {expanded.has(schema.name) && (
            <div className="ml-2">
              {(schema.tables ?? []).map((table) => (
                <button
                  key={`${schema.name}.${table.name}`}
                  onClick={() =>
                    navigate(
                      `/connections/${id}/browse?schema=${encodeURIComponent(
                        schema.name
                      )}&table=${encodeURIComponent(table.name)}`
                    )
                  }
                  className="w-full flex items-center gap-2 px-3 py-1 text-sm text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
                >
                  <span className="text-xs">
                    {table.type === 'view' ? '👁' : table.type === 'collection' ? '📦' : '📄'}
                  </span>
                  <span className="truncate">{table.name}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
