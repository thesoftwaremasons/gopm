import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createConnection } from '../api/client'
import type { CreateConnectionRequest } from '../types'

interface Props {
  onClose: () => void
}

export function ConnectionForm({ onClose }: Props) {
  const qc = useQueryClient()
  const [form, setForm] = useState({
    name: '',
    engine: 'postgres',
    host: 'localhost',
    port: 5432,
    database: '',
    username: '',
    password: '',
    ssl_mode: 'disable',
    read_only: true,
  })

  const mutation = useMutation({
    mutationFn: (data: CreateConnectionRequest) => createConnection(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connections'] })
      onClose()
    },
  })

  const handleEngineChange = (engine: string) => {
    setForm((f) => ({
      ...f,
      engine,
      port: engine === 'postgres' ? 5432 : 27017,
    }))
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    mutation.mutate({
      name: form.name,
      engine: form.engine,
      config: {
        host: form.host,
        port: form.port,
        database: form.database,
        username: form.username,
        password: form.password,
        ssl_mode: form.ssl_mode,
        read_only: form.read_only,
      },
    })
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4">Add Connection</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
            <input
              className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              required
              placeholder="My Database"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Engine</label>
            <select
              className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={form.engine}
              onChange={(e) => handleEngineChange(e.target.value)}
            >
              <option value="postgres">PostgreSQL</option>
              <option value="mongodb">MongoDB</option>
            </select>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">
              <label className="block text-sm font-medium text-gray-700 mb-1">Host</label>
              <input
                className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                value={form.host}
                onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))}
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Port</label>
              <input
                type="number"
                className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                value={form.port}
                onChange={(e) => setForm((f) => ({ ...f, port: Number(e.target.value) }))}
                required
              />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Database</label>
            <input
              className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={form.database}
              onChange={(e) => setForm((f) => ({ ...f, database: e.target.value }))}
              required
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Username</label>
              <input
                className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                value={form.username}
                onChange={(e) => setForm((f) => ({ ...f, username: e.target.value }))}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
              <input
                type="password"
                className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                value={form.password}
                onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
              />
            </div>
          </div>

          {form.engine === 'postgres' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">SSL Mode</label>
              <select
                className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                value={form.ssl_mode}
                onChange={(e) => setForm((f) => ({ ...f, ssl_mode: e.target.value }))}
              >
                <option value="disable">Disable</option>
                <option value="require">Require</option>
                <option value="verify-full">Verify Full</option>
              </select>
            </div>
          )}

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="read_only"
              checked={form.read_only}
              onChange={(e) => setForm((f) => ({ ...f, read_only: e.target.checked }))}
              className="rounded border-gray-300"
            />
            <label htmlFor="read_only" className="text-sm text-gray-700">
              Read-only (blocks INSERT/UPDATE/DELETE)
            </label>
          </div>

          {mutation.error && (
            <div className="text-red-600 text-sm bg-red-50 rounded p-2">
              {(mutation.error as Error).message}
            </div>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm text-gray-600 hover:text-gray-800"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="px-4 py-2 text-sm bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50"
            >
              {mutation.isPending ? 'Saving...' : 'Save Connection'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
