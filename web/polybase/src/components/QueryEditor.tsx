import React, { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { ResultGrid } from './ResultGrid'
import type { Connection, ResultSet } from '../types'

interface Props {
  connection: Connection
}

export function QueryEditor({ connection }: Props) {
  const [queryText, setQueryText] = useState('')
  const [pendingQuery, setPendingQuery] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery<ResultSet>({
    queryKey: ['query', connection.id, pendingQuery],
    queryFn: () => api.query(connection.id, pendingQuery!),
    enabled: pendingQuery !== null && pendingQuery.trim().length > 0,
  })

  const run = useCallback(() => {
    if (queryText.trim()) setPendingQuery(queryText)
  }, [queryText])

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); run() }
  }

  return (
    <div className="flex flex-col gap-3 p-4 h-full">
      <div className="flex items-center gap-3">
        <h3 className="text-sm font-medium text-white">Query Editor</h3>
        {connection.read_only && (
          <span className="px-2 py-0.5 rounded text-xs bg-yellow-900 text-yellow-300">Read-only enforced</span>
        )}
        <span className="ml-auto text-xs text-gray-500">
          {connection.engine === 'mongodb' ? 'JSON find filter' : 'SQL'}
        </span>
      </div>
      <textarea value={queryText} onChange={e => setQueryText(e.target.value)} onKeyDown={handleKeyDown}
        placeholder={connection.engine === 'mongodb' ? '{"field": "value"}' : 'SELECT * FROM public.users LIMIT 100'}
        rows={6}
        className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm font-mono resize-y focus:outline-none focus:border-blue-500" />
      <div className="flex gap-2">
        <button onClick={run} disabled={isLoading || !queryText.trim()}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded text-sm disabled:opacity-40">
          {isLoading ? 'Running…' : '▶ Run (Ctrl+Enter)'}
        </button>
        <button onClick={() => { setQueryText(''); setPendingQuery(null) }}
          className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded text-sm">Clear</button>
      </div>
      {error && <div className="bg-red-900/50 text-red-300 text-sm rounded px-3 py-2">{(error as Error).message}</div>}
      {data && <ResultGrid result={data} />}
    </div>
  )
}
