import React, { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { ResultGrid } from './ResultGrid'
import type { ResultSet } from '../types'

interface Props {
  connectionId: string
  schema: string
  table: string
}

export function DataBrowser({ connectionId, schema, table }: Props) {
  const [offset, setOffset] = useState(0)
  const limit = 100

  const { data, isLoading, error } = useQuery<ResultSet>({
    queryKey: ['browse', connectionId, schema, table, offset],
    queryFn: () => api.browse(connectionId, schema, table, offset, limit),
  })

  return (
    <div className="flex flex-col gap-3 p-4">
      <div className="flex items-center gap-2">
        <span className="text-xs text-gray-500">Schema:</span>
        <span className="font-mono text-sm text-gray-300">{schema}</span>
        <span className="text-xs text-gray-500 ml-2">Table:</span>
        <span className="font-mono text-sm text-white font-medium">{table}</span>
      </div>
      {isLoading && <p className="text-gray-400 text-sm">Loading…</p>}
      {error && <p className="text-red-400 text-sm">{(error as Error).message}</p>}
      {data && <ResultGrid result={data} offset={offset} limit={limit} onPageChange={setOffset} />}
    </div>
  )
}
