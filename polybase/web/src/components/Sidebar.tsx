import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ConnectionList } from './ConnectionList'
import { ConnectionForm } from './ConnectionForm'
import { SchemaTree } from './SchemaTree'
import { deleteConnection, testConnection } from '../api/client'

export function Sidebar() {
  const [showForm, setShowForm] = useState(false)
  const [testStatus, setTestStatus] = useState<{ ok: boolean; msg: string } | null>(null)
  const { id } = useParams<{ id: string }>()
  const qc = useQueryClient()

  const deleteMutation = useMutation({
    mutationFn: deleteConnection,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }),
  })

  const testMutation = useMutation({
    mutationFn: testConnection,
    onSuccess: (data) => {
      setTestStatus({ ok: data.ok, msg: data.ok ? 'Connection successful' : (data.error ?? 'Connection failed') })
      setTimeout(() => setTestStatus(null), 4000)
    },
    onError: (err: Error) => {
      setTestStatus({ ok: false, msg: err.message })
      setTimeout(() => setTestStatus(null), 4000)
    },
  })

  return (
    <aside className="w-64 bg-slate-800 flex flex-col h-full text-white">
      {/* Logo */}
      <div className="px-4 py-4 border-b border-slate-700">
        <h1 className="text-lg font-bold text-white tracking-tight">
          🗄 Polybase
        </h1>
        <p className="text-xs text-slate-400 mt-0.5">DBMS Browser</p>
      </div>

      {/* Connections list */}
      <div className="flex-1 overflow-y-auto py-2">
        <ConnectionList />

        {/* Schema tree when a connection is selected */}
        {id && (
          <>
            <div className="border-t border-slate-700 mt-2 pt-2">
              <SchemaTree />
            </div>

            {/* Per-connection actions */}
            <div className="px-3 py-2 mt-2 border-t border-slate-700 space-y-1">
              <Link
                to={`/connections/${id}/query`}
                className="block w-full text-left px-2 py-1.5 text-xs text-slate-300 hover:bg-slate-700 rounded transition-colors"
              >
                ⚡ Query Editor
              </Link>
              <button
                onClick={() => testMutation.mutate(id)}
                disabled={testMutation.isPending}
                className="w-full text-left px-2 py-1.5 text-xs text-slate-300 hover:bg-slate-700 rounded transition-colors disabled:opacity-50"
              >
                {testMutation.isPending ? '⏳ Testing...' : '✓ Test Connection'}
              </button>
              {testStatus && (
                <div
                  className={`px-2 py-1.5 text-xs rounded ${
                    testStatus.ok
                      ? 'bg-green-800 text-green-100'
                      : 'bg-red-800 text-red-100'
                  }`}
                >
                  {testStatus.msg}
                </div>
              )}
              <button
                onClick={() => {
                  if (confirm('Delete this connection?')) {
                    deleteMutation.mutate(id)
                  }
                }}
                className="w-full text-left px-2 py-1.5 text-xs text-red-400 hover:bg-slate-700 rounded transition-colors"
              >
                🗑 Delete Connection
              </button>
            </div>
          </>
        )}
      </div>

      {/* Add connection button */}
      <div className="p-3 border-t border-slate-700">
        <button
          onClick={() => setShowForm(true)}
          className="w-full px-3 py-2 bg-blue-500 hover:bg-blue-600 text-white text-sm rounded transition-colors font-medium"
        >
          + Add Connection
        </button>
      </div>

      {showForm && <ConnectionForm onClose={() => setShowForm(false)} />}
    </aside>
  )
}
