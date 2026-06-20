import React from 'react'
import { Routes, Route, Navigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Sidebar } from './components/Sidebar'
import { ConnectionList } from './components/ConnectionList'
import { DataBrowser } from './components/DataBrowser'
import { QueryEditor } from './components/QueryEditor'
import { api } from './api/client'
import type { Connection } from './types'

function BrowseRoute() {
  const { connId, schema, table } = useParams<{ connId: string; schema: string; table: string }>()
  if (!connId || !schema || !table) return <Navigate to="/" />
  return (
    <DataBrowser
      connectionId={connId}
      schema={decodeURIComponent(schema)}
      table={decodeURIComponent(table)}
    />
  )
}

function QueryRoute() {
  const { connId } = useParams<{ connId: string }>()
  const { data: conn } = useQuery<Connection[], Error, Connection | undefined>({
    queryKey: ['connections'],
    queryFn: api.listConnections,
    select: (cs: Connection[]) => cs.find((c: Connection) => c.id === connId),
  })
  if (!connId) return <Navigate to="/" />
  if (!conn) return <p className="text-gray-400 p-4">Connection not found.</p>
  return <QueryEditor connection={conn} />
}

export default function App() {
  return (
    <div className="flex h-screen bg-gray-950 text-white overflow-hidden">
      <Sidebar />
      <main className="flex-1 overflow-auto">
        <Routes>
          <Route path="/" element={<Navigate to="/connections" />} />
          <Route path="/connections" element={<ConnectionList />} />
          <Route path="/browse/:connId/:schema/:table" element={<BrowseRoute />} />
          <Route path="/query/:connId" element={<QueryRoute />} />
        </Routes>
      </main>
    </div>
  )
}
