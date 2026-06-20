import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { Connection } from '../types'
import { api } from '../api/client'
import { ConnectionForm } from './ConnectionForm'

export function ConnectionList() {
  const qc = useQueryClient()
  const { data: conns = [], isLoading, error } = useQuery<Connection[]>({
    queryKey: ['connections'],
    queryFn: api.listConnections,
  })
  const deleteMut = useMutation({
    mutationFn: api.deleteConnection,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }),
  })

  const [showForm, setShowForm] = useState(false)
  const [editConn, setEditConn] = useState<Connection | undefined>()

  function openNew() { setEditConn(undefined); setShowForm(true) }
  function openEdit(c: Connection) { setEditConn(c); setShowForm(true) }
  function onSaved() { setShowForm(false); qc.invalidateQueries({ queryKey: ['connections'] }) }

  if (isLoading) return <p className="text-gray-400 p-4">Loading…</p>
  if (error) return <p className="text-red-400 p-4">{(error as Error).message}</p>

  return (
    <div className="p-6 max-w-4xl">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-semibold text-white">Connections</h2>
        <button onClick={openNew} className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded text-sm">+ New Connection</button>
      </div>
      {showForm && (
        <div className="mb-6 bg-gray-800 rounded-lg p-6 border border-gray-700">
          <h3 className="text-lg font-medium text-white mb-4">{editConn ? 'Edit Connection' : 'New Connection'}</h3>
          <ConnectionForm existing={editConn} onSaved={onSaved} onCancel={() => setShowForm(false)} />
        </div>
      )}
      {conns.length === 0
        ? <p className="text-gray-500">No connections yet. Add one to get started.</p>
        : (
          <div className="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
            <table className="w-full text-sm text-left text-gray-300">
              <thead className="bg-gray-700/50 text-xs text-gray-400 uppercase">
                <tr>
                  <th className="px-4 py-3">Name</th><th className="px-4 py-3">Engine</th>
                  <th className="px-4 py-3">Host</th><th className="px-4 py-3">Database</th>
                  <th className="px-4 py-3">Read Only</th><th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {conns.map((c: Connection) => (
                  <tr key={c.id} className="border-t border-gray-700 hover:bg-gray-700/30">
                    <td className="px-4 py-3 font-medium text-white">{c.name}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded text-xs ${c.engine === 'postgres' ? 'bg-blue-900 text-blue-300' : 'bg-green-900 text-green-300'}`}>
                        {c.engine}
                      </span>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">{c.host}:{c.port}</td>
                    <td className="px-4 py-3 font-mono text-xs">{c.database}</td>
                    <td className="px-4 py-3">
                      {c.read_only && <span className="px-2 py-0.5 rounded text-xs bg-yellow-900 text-yellow-300">read-only</span>}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-2">
                        <button onClick={() => openEdit(c)} className="text-xs px-2 py-1 rounded bg-gray-700 hover:bg-gray-600">Edit</button>
                        <button onClick={() => { if (window.confirm(`Delete "${c.name}"?`)) deleteMut.mutate(c.id) }}
                          className="text-xs px-2 py-1 rounded bg-red-900/50 hover:bg-red-900 text-red-400">Delete</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      }
    </div>
  )
}
