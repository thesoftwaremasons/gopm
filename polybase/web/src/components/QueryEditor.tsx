import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { runQuery } from '../api/client'
import { ResultGrid } from './ResultGrid'
import type { ResultSet } from '../types'

export function QueryEditor() {
  const { id } = useParams<{ id: string }>()
  const [query, setQuery] = useState('')
  const [result, setResult] = useState<ResultSet | null>(null)

  const mutation = useMutation({
    mutationFn: ({ q, limit }: { q: string; limit: number }) => runQuery(id!, q, limit),
    onSuccess: (data) => setResult(data),
  })

  const handleRun = () => {
    if (!query.trim() || !id) return
    mutation.mutate({ q: query, limit: 1000 })
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      handleRun()
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-white">
        <h2 className="font-semibold text-gray-800">Query Editor</h2>
        <button
          onClick={handleRun}
          disabled={mutation.isPending || !query.trim()}
          className="px-4 py-1.5 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 disabled:opacity-50 transition-colors"
        >
          {mutation.isPending ? 'Running...' : 'Run (Ctrl+Enter)'}
        </button>
      </div>

      {/* Editor area */}
      <div className="flex flex-col" style={{ height: '40%' }}>
        <textarea
          className="flex-1 w-full p-4 font-mono text-sm resize-none border-b focus:outline-none focus:ring-2 focus:ring-inset focus:ring-blue-200"
          placeholder={
            id
              ? 'Enter your query here...\nFor PostgreSQL: SELECT * FROM users\nFor MongoDB: collection_name\n{"field": "value"}'
              : 'Select a connection first'
          }
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={!id}
        />
      </div>

      {/* Results */}
      <div className="flex-1 overflow-hidden">
        {mutation.error && (
          <div className="p-4 text-red-600 bg-red-50 border-b">
            Error: {(mutation.error as Error).message}
          </div>
        )}
        {result && !mutation.error ? (
          <ResultGrid result={result} />
        ) : !mutation.isPending && !mutation.error ? (
          <div className="flex items-center justify-center h-full text-gray-400">
            <div className="text-center">
              <div className="text-3xl mb-2">⚡</div>
              <div className="text-sm">Results will appear here</div>
              <div className="text-xs mt-1 text-gray-300">Press Ctrl+Enter to run</div>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
}
