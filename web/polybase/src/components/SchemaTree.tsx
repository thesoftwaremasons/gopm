import React, { useState } from 'react'
import type { SchemaInfo } from '../types'

interface Props {
  schemas: SchemaInfo[]
  onSelectTable: (schema: string, table: string) => void
}

export function SchemaTree({ schemas, onSelectTable }: Props) {
  const [openSchemas, setOpenSchemas] = useState<Set<string>>(new Set())

  function toggle(name: string) {
    setOpenSchemas(prev => {
      const next = new Set(prev)
      next.has(name) ? next.delete(name) : next.add(name)
      return next
    })
  }

  if (!schemas.length) return <p className="text-gray-500 text-xs px-2">No schemas found.</p>

  return (
    <div className="text-sm">
      {schemas.map(s => (
        <div key={s.name}>
          <button onClick={() => toggle(s.name)}
            className="w-full text-left px-2 py-1 flex items-center gap-1 text-gray-300 hover:bg-gray-700 rounded">
            <span>{openSchemas.has(s.name) ? '▾' : '▸'}</span>
            <span className="font-mono">{s.name}</span>
            <span className="ml-auto text-xs text-gray-500">{s.tables?.length ?? 0}</span>
          </button>
          {openSchemas.has(s.name) && (
            <div className="ml-4 border-l border-gray-700 pl-2">
              {(s.tables ?? []).map(t => (
                <button key={t.name} onClick={() => onSelectTable(s.name, t.name)}
                  className="w-full text-left px-2 py-0.5 text-gray-400 hover:text-white hover:bg-gray-700 rounded flex items-center gap-1">
                  <span className="text-xs text-gray-600">
                    {t.type === 'view' ? '◈' : t.type === 'collection' ? '◉' : '▣'}
                  </span>
                  <span className="font-mono text-xs">{t.name}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
