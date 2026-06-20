import React, { useState } from 'react'
import type { Connection, ConnectionFormData } from '../types'
import { api } from '../api/client'

interface Props {
  existing?: Connection
  onSaved: () => void
  onCancel: () => void
}

const defaultForm: ConnectionFormData = {
  name: '', engine: 'postgres', host: 'localhost', port: 5432,
  database: '', username: '', password: '', read_only: false, ssl_mode: 'prefer',
}

export function ConnectionForm({ existing, onSaved, onCancel }: Props) {
  const [form, setForm] = useState<ConnectionFormData>(() =>
    existing ? {
      name: existing.name, engine: existing.engine, host: existing.host,
      port: existing.port, database: existing.database, username: existing.username,
      password: '', read_only: existing.read_only, ssl_mode: existing.ssl_mode ?? 'prefer',
    } : defaultForm
  )
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testMsg, setTestMsg] = useState<{ ok: boolean; msg: string } | null>(null)
  const [error, setError] = useState<string | null>(null)

  function set<K extends keyof ConnectionFormData>(k: K, v: ConnectionFormData[K]) {
    setForm((f: ConnectionFormData) => ({ ...f, [k]: v }))
  }

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true); setError(null)
    try {
      if (existing) { await api.updateConnection(existing.id, form) }
      else { await api.createConnection(form) }
      onSaved()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally { setSaving(false) }
  }

  async function handleTest() {
    if (!existing) { setTestMsg({ ok: false, msg: 'Save the connection first to test it.' }); return }
    setTesting(true); setTestMsg(null)
    try {
      await api.testConnection(existing.id)
      setTestMsg({ ok: true, msg: 'Connection successful!' })
    } catch (err: unknown) {
      setTestMsg({ ok: false, msg: err instanceof Error ? err.message : String(err) })
    } finally { setTesting(false) }
  }

  return (
    <form onSubmit={handleSave} className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-4">
        <div className="col-span-2">
          <label className="block text-xs text-gray-400 mb-1">Name *</label>
          <input required value={form.name} onChange={e => set('name', e.target.value)}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm" placeholder="My Database" />
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Engine *</label>
          <select value={form.engine} onChange={e => { const eng = e.target.value as 'postgres' | 'mongodb'; set('engine', eng); set('port', eng === 'mongodb' ? 27017 : 5432) }}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm">
            <option value="postgres">PostgreSQL</option>
            <option value="mongodb">MongoDB</option>
          </select>
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Host *</label>
          <input required value={form.host} onChange={e => set('host', e.target.value)}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm" />
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Port *</label>
          <input required type="number" value={form.port} onChange={e => set('port', Number(e.target.value))}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm" />
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Database *</label>
          <input required value={form.database} onChange={e => set('database', e.target.value)}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm" />
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Username</label>
          <input value={form.username} onChange={e => set('username', e.target.value)}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm" />
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Password</label>
          <input type="password" value={form.password} onChange={e => set('password', e.target.value)}
            className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm"
            placeholder={existing ? '(unchanged)' : ''} />
        </div>
        {form.engine === 'postgres' && (
          <div>
            <label className="block text-xs text-gray-400 mb-1">SSL Mode</label>
            <select value={form.ssl_mode} onChange={e => set('ssl_mode', e.target.value)}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white text-sm">
              {['disable','allow','prefer','require','verify-ca','verify-full'].map(m => <option key={m} value={m}>{m}</option>)}
            </select>
          </div>
        )}
        <div className="flex items-center gap-2 col-span-2">
          <input type="checkbox" id="readonly" checked={form.read_only} onChange={e => set('read_only', e.target.checked)} className="w-4 h-4" />
          <label htmlFor="readonly" className="text-sm text-gray-300">Read-only connection</label>
        </div>
      </div>
      {testMsg && <div className={`text-sm px-3 py-2 rounded ${testMsg.ok ? 'bg-green-900/50 text-green-300' : 'bg-red-900/50 text-red-300'}`}>{testMsg.msg}</div>}
      {error && <div className="text-sm px-3 py-2 rounded bg-red-900/50 text-red-300">{error}</div>}
      <div className="flex gap-2 justify-end">
        <button type="button" onClick={handleTest} disabled={testing || !existing}
          className="px-4 py-2 rounded bg-gray-700 text-sm hover:bg-gray-600 disabled:opacity-40">
          {testing ? 'Testing…' : 'Test Connection'}
        </button>
        <button type="button" onClick={onCancel} className="px-4 py-2 rounded bg-gray-700 text-sm hover:bg-gray-600">Cancel</button>
        <button type="submit" disabled={saving} className="px-4 py-2 rounded bg-blue-600 text-sm hover:bg-blue-500 disabled:opacity-40">
          {saving ? 'Saving…' : existing ? 'Update' : 'Create'}
        </button>
      </div>
    </form>
  )
}
