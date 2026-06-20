import React, { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import type { Connection, SchemaInfo } from '../types'
import { api } from '../api/client'
import { SchemaTree } from './SchemaTree'

export function Sidebar() {
  const navigate = useNavigate()
  const { data: conns = [] } = useQuery<Connection[]>({
    queryKey: ['connections'],
    queryFn: api.listConnections,
    refetchInterval: 10000,
  })

  const [expandedConn, setExpandedConn] = useState<string | null>(null)

  function toggleConn(id: string) {
    setExpandedConn(prev => prev === id ? null : id)
  }

  return (
    <aside className="w-64 bg-gray-900 border-r border-gray-800 flex flex-col h-full">
      <div className="px-4 py-4 border-b border-gray-800">
        <Link to="/" className="text-lg font-bold text-white flex items-center gap-2">
          <span className="text-blue-400">⬡</span> Polybase
        </Link>
      </div>
      <nav className="px-2 py-2 border-b border-gray-800">
        <Link to="/connections" className="block px-3 py-2 rounded text-sm text-gray-300 hover:bg-gray-700 hover:text-white">
          🔌 Connections
        </Link>
      </nav>
      <div className="flex-1 overflow-y-auto py-2">
        {conns.length === 0 && <p className="text-xs text-gray-600 px-4 py-2">No connections</p>}
        {conns.map((c: Connection) => (
          <div key={c.id}>
            <button onClick={() => toggleConn(c.id)}
              className="w-full text-left px-3 py-2 flex items-center gap-2 text-sm text-gray-300 hover:bg-gray-800 hover:text-white">
              <span className={`text-xs ${c.engine === 'postgres' ? 'text-blue-400' : 'text-green-400'}`}>
                {c.engine === 'postgres' ? '🐘' : '🍃'}
              </span>
              <span className="truncate">{c.name}</span>
              <span className="ml-auto text-xs text-gray-600">{expandedConn === c.id ? '▾' : '▸'}</span>
            </button>
            {expandedConn === c.id && (
              <ExpandedConnection conn={c}
                onSelectTable={(schema, table) => navigate(`/browse/${c.id}/${encodeURIComponent(schema)}/${encodeURIComponent(table)}`)} />
            )}
          </div>
        ))}
      </div>
    </aside>
  )
}

function ExpandedConnection({ conn, onSelectTable }: { conn: Connection; onSelectTable: (schema: string, table: string) => void }) {
  const { data: schemas = [], isLoading, error } = useQuery<SchemaInfo[]>({
    queryKey: ['schemas', conn.id],
    queryFn: () => api.listSchemas(conn.id),
  })

  if (isLoading) return <p className="text-xs text-gray-500 px-6 py-1">Loading schemas…</p>
  if (error) return <p className="text-xs text-red-400 px-6 py-1">Error loading schemas</p>

  return (
    <div className="pl-4 pb-1">
      <SchemaTree schemas={schemas} onSelectTable={onSelectTable} />
      <Link to={`/query/${conn.id}`} className="block px-2 py-1 text-xs text-gray-500 hover:text-gray-300">✎ Query Editor</Link>
    </div>
  )
}
