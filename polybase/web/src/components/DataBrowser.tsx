import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams, useSearchParams } from 'react-router-dom'
import { browseTable } from '../api/client'
import { ResultGrid } from './ResultGrid'

const LIMIT = 100

export function DataBrowser() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const schema = searchParams.get('schema') ?? ''
  const table = searchParams.get('table') ?? ''
  const [offset, setOffset] = useState(0)

  const { data, isLoading, error } = useQuery({
    queryKey: ['browse', id, schema, table, offset],
    queryFn: () => browseTable(id!, schema, table, offset, LIMIT),
    enabled: !!id && !!table,
  })

  if (!table) {
    return (
      <div className="flex items-center justify-center h-full text-gray-400">
        <div className="text-center">
          <div className="text-4xl mb-3">📊</div>
          <div className="text-lg font-medium">Select a table to browse</div>
          <div className="text-sm mt-1">Click a table in the sidebar to view its data</div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-2 px-4 py-3 border-b bg-white">
        <span className="text-gray-500 text-sm">{schema}</span>
        {schema && <span className="text-gray-400">.</span>}
        <span className="font-semibold text-gray-800">{table}</span>
        {isLoading && (
          <span className="ml-2 text-xs text-blue-500 animate-pulse">Loading...</span>
        )}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden">
        {error ? (
          <div className="p-6 text-red-600">
            Error: {(error as Error).message}
          </div>
        ) : isLoading ? (
          <div className="p-6 text-gray-500">Loading data...</div>
        ) : data ? (
          <ResultGrid
            result={data}
            offset={offset}
            limit={LIMIT}
            onPageChange={setOffset}
          />
        ) : null}
      </div>
    </div>
  )
}
